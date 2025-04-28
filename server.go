package main

import (
	"context"
	"fmt"
	"net"

	"github.com/miekg/dns"
	"go.uber.org/zap"
)

var (
	ipv4MulticastAddr        = net.ParseIP("224.0.0.251")
	mdnsPort                 = 5353
	defaultTTL        uint32 = 120
	udpMaxBuffSize           = 65536
)

type MDNSServer struct {
	names  map[string]struct{}
	logger *zap.Logger
}

func NewMDNSServer(names []string, logger *zap.Logger) *MDNSServer {
	namesMap := make(map[string]struct{})
	for _, name := range names {
		namesMap[name] = struct{}{}
	}

	return &MDNSServer{
		names:  namesMap,
		logger: logger,
	}
}

func (s *MDNSServer) Start(ctx context.Context, bindInterface *net.Interface, localIPAddress net.IP) error {
	ipv4Listener, err := net.ListenMulticastUDP("udp4", bindInterface, &net.UDPAddr{IP: ipv4MulticastAddr, Port: mdnsPort})
	if err != nil {
		return fmt.Errorf("failed to create ipv4 listener: %w", err)
	}

	return s.handle(ctx, ipv4Listener, localIPAddress)
}

func (s *MDNSServer) handle(ctx context.Context, conn *net.UDPConn, answerAddress net.IP) error {
	buf := make([]byte, udpMaxBuffSize)
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("context was done. Extiting...")
			return nil

		default:
			n, from, err := conn.ReadFrom(buf)
			if err != nil {
				continue
			}

			var msg dns.Msg
			err = msg.Unpack(buf[:n])
			if err != nil {
				s.logger.Error("failed to unpack message", zap.Error(err))
				continue
			}

			var rr []dns.RR

			for _, q := range msg.Question {
				s.logger.Debug("receive query", zap.String("query", q.Name))
				if q.Qtype != dns.TypeA {
					continue
				}

				_, ok := s.names[q.Name]
				if !ok {
					continue
				}

				s.logger.Info("handle query", zap.String("query", q.Name))

				rr = append(rr, &dns.A{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    defaultTTL,
					},
					A: answerAddress,
				})
			}

			if len(rr) == 0 {
				continue
			}

			answer := dns.Msg{
				MsgHdr: dns.MsgHdr{
					Id:            msg.Id,
					Response:      true,
					Opcode:        dns.OpcodeQuery,
					Authoritative: true,
				},
				Compress: true,
				Answer:   rr,
			}

			buf, err := answer.Pack()
			if err != nil {
				s.logger.Error("failed to pack answer", zap.Error(err))
				continue
			}

			_, err = conn.WriteToUDP(buf, from.(*net.UDPAddr))
			if err != nil {
				s.logger.Error("failer to send answer", zap.Error(err))
			}
		}
	}
}
