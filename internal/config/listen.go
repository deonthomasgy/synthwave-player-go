package config

import (
	"fmt"
	"strings"
)

// NormalizeListenHost returns the address the HTTP server binds to.
// Empty, "*", and "0.0.0.0" mean all IPv4 interfaces.
func NormalizeListenHost(host string) string {
	host = strings.TrimSpace(host)
	switch host {
	case "", "*", "0.0.0.0":
		return "0.0.0.0"
	default:
		return host
	}
}

// ListenAddr formats host:port for net/http Listen.
func ListenAddr(host string, port int) string {
	host = NormalizeListenHost(host)
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		return fmt.Sprintf("[%s]:%d", host, port)
	}
	return fmt.Sprintf("%s:%d", host, port)
}
