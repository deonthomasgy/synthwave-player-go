package network

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/huin/goupnp"
	"github.com/huin/goupnp/dcps/internetgateway1"
)

func SetupUPnP(port int, logger *slog.Logger) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clients, _, err := internetgateway1.NewWANIPConnection1Clients()
	if err != nil {
		return "UPnP discovery failed: " + err.Error()
	}
	_ = ctx
	if len(clients) == 0 {
		// Try discovery via control point
		devs, err := goupnp.DiscoverDevices("")
		if err != nil || len(devs) == 0 {
			return fmt.Sprintf("No UPnP-enabled gateway was found. You may need to manually forward port %d (TCP) on your router.", port)
		}
	}

	localIP := localOutboundIP()
	if localIP == "" {
		return "Could not determine local IP address for UPnP."
	}

	for _, client := range clients {
		err := client.AddPortMapping(
			"",
			uint16(port),
			"TCP",
			uint16(port),
			localIP,
			true,
			"Synthwave Music Player",
			0,
		)
		if err != nil {
			logger.Error("UPnP port mapping failed", "err", err)
			continue
		}
		logger.Info("UPnP port forwarding configured", "port", port)
		return ""
	}
	return fmt.Sprintf("UPnP port forwarding failed. You may need to manually forward port %d (TCP) on your router.", port)
}

func localOutboundIP() string {
	conn, err := net.DialTimeout("udp", "8.8.8.8:53", 2*time.Second)
	if err != nil {
		return ""
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}
