package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/chmike/domain"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewProduction()

	names, err := getNames()
	if err != nil {
		logger.Fatal("failed to parse names", zap.Error(err))
	}

	if len(names) == 0 {
		log.Fatal("no names for publish")
	}

	// MDNS_PUB_BIND_IFACE
	var bindInterface *net.Interface
	bindInterfaceName := os.Getenv("MDNS_PUB_BIND_IFACE")
	if bindInterfaceName == "" {
		logger.Fatal("MDNS_PUB_BIND_IFACE (bind interface) is required")
	}

	iface, err := net.InterfaceByName(bindInterfaceName)
	if err != nil {
		logger.Fatal("failed to get interface to bind", zap.Error(err))
	}

	bindInterface = iface

	// MDNS_PUB_LOCAL_IP
	var localIPAddress net.IP

	localIPAddressConfig := os.Getenv("MDNS_PUB_LOCAL_IP")
	if localIPAddressConfig != "" {
		localIPAddress = net.ParseIP(localIPAddressConfig)
		if localIPAddress == nil {
			logger.Fatal(fmt.Sprintf("failed to parse MDNS_PUB_LOCAL_IP (%s)", localIPAddressConfig))
		}
	}

	// MDNS_PUB_LOCAL_IFACE
	var localIPInterface *net.Interface
	localIPInterfaceName := os.Getenv("MDNS_PUB_LOCAL_IFACE")
	if localIPInterfaceName != "" && localIPAddress == nil {
		iface, err := net.InterfaceByName(bindInterfaceName)
		if err != nil {
			log.Fatal("failed to get local ip interface", zap.Error(err))
		}

		localIPInterface = iface

		localIPAddress, err = getFirstInterfaceIPAddress(*localIPInterface)
		if err != nil {
			logger.Fatal(fmt.Sprintf("failed to get ip address of interface %s", localIPInterface.Name), zap.Error(err))
		}
	}

	// default local ip address
	if localIPAddress == nil {
		localIPAddress, err = getDefaultRouteIPAddress()
		if err != nil {
			logger.Fatal("failed to get default ip address", zap.Error(err))
		}
	}

	if localIPAddress == nil {
		logger.Fatal("failed to get local ip address")
	}

	logger.Info(fmt.Sprintf("local ip address: %s", localIPAddress))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srv := NewMDNSServer(names, logger)

	err = srv.Start(ctx, bindInterface, localIPAddress)
	if err != nil {
		logger.Fatal("", zap.Error(err))
	}
}

func getNames() ([]string, error) {
	var result []string
	var err error

	names := os.Getenv("MDNS_PUB_NAMES")
	for _, name := range strings.Split(names, ";") {
		err = domain.Check(name)
		if err != nil {
			return result, err
		}
		if !strings.HasSuffix(name, ".") {
			name += "."
		}

		result = append(result, name)
	}

	return result, err
}

func getFirstInterfaceIPAddress(iface net.Interface) (net.IP, error) {
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, err
	}

	var ipv4Addr net.IP

	for _, addr := range addrs {
		if ipv4Addr = addr.(*net.IPNet).IP.To4(); ipv4Addr != nil {
			break
		}
	}
	if ipv4Addr == nil {
		return nil, fmt.Errorf("interface %s don't have an ipv4 address", iface.Name)
	}
	return ipv4Addr, nil
}

func getDefaultRouteIPAddress() (net.IP, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP, nil
}
