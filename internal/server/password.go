package server

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"regexp"
	"strings"

	"github.com/Elive/synthwave-player/internal/config"
	"github.com/Elive/synthwave-player/internal/network"
)

var hex64 = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

// resolvePasswordInput accepts a client SHA-256 hex hash, or plaintext on private networks
// when the browser cannot use crypto.subtle (HTTP via LAN IP, not localhost/HTTPS).
func resolvePasswordInput(r *http.Request, input string, privateNets []string) (string, bool) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", false
	}
	if hex64.MatchString(input) {
		return strings.ToLower(input), true
	}
	addr := network.RemoteAddrWithPort(r)
	if !network.IsPrivateIP(addr, privateNets) {
		return "", false
	}
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:]), true
}

func resolvePasswordInputFromConfig(r *http.Request, input string, cfg *config.Store) (string, bool) {
	return resolvePasswordInput(r, input, cfg.Get().PrivateNetworks)
}
