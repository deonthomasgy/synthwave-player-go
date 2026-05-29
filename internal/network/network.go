package network

import (
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Elive/synthwave-player/internal/config"
)

type RateLimiter struct {
	mu          sync.Mutex
	ipLimits    map[string]*ipBucket
	global      bucket
	lastCleanup time.Time
	cfg         config.Settings
}

type bucket struct {
	count      int
	lastReset  time.Time
}

type ipBucket struct {
	count     int
	lastReset time.Time
}

func NewRateLimiter(cfg config.Settings) *RateLimiter {
	return &RateLimiter{
		ipLimits: make(map[string]*ipBucket),
		cfg:      cfg,
	}
}

func (r *RateLimiter) Allow(ip, address string, privateNets []string) (bool, string) {
	if IsPrivateIP(address, privateNets) {
		return true, ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	if now.Sub(r.lastCleanup) > 5*time.Minute {
		for k, v := range r.ipLimits {
			if now.Sub(v.lastReset) > time.Duration(r.cfg.RateLimitWindow)*time.Second {
				delete(r.ipLimits, k)
			}
		}
		r.lastCleanup = now
	}
	win := time.Duration(r.cfg.GlobalRateLimitWindow) * time.Second
	if now.Sub(r.global.lastReset) > win {
		r.global.count = 0
		r.global.lastReset = now
	}
	if r.global.count >= r.cfg.GlobalRateLimitRequests {
		return false, fmt.Sprintf("Global rate limit exceeded. Max %d requests per %d seconds allowed.", r.cfg.GlobalRateLimitRequests, r.cfg.GlobalRateLimitWindow)
	}
	b, ok := r.ipLimits[ip]
	if !ok {
		if len(r.ipLimits) >= r.cfg.MaxSimultaneousIPs {
			return false, "The server is currently at maximum capacity. Please try again later."
		}
		b = &ipBucket{lastReset: now}
		r.ipLimits[ip] = b
	}
	ipWin := time.Duration(r.cfg.RateLimitWindow) * time.Second
	if now.Sub(b.lastReset) > ipWin {
		b.count = 0
		b.lastReset = now
	}
	if b.count >= r.cfg.RateLimitRequests {
		return false, "Your IP has been rate-limited for security reasons / to avoid bots. Please try again in a moment."
	}
	b.count++
	r.global.count++
	return true, ""
}

var (
	regexCache sync.Map
	ipCache    sync.Map
)

func CompileRegex(pattern string) (*regexp.Regexp, error) {
	if v, ok := regexCache.Load(pattern); ok {
		if v == nil {
			return nil, fmt.Errorf("invalid regex")
		}
		return v.(*regexp.Regexp), nil
	}
	if len(pattern) > 2 && pattern[0] == '/' {
		for i := len(pattern) - 1; i > 0; i-- {
			if pattern[i] == '/' {
				flags := pattern[i+1:]
				body := pattern[1:i]
				re, err := regexp.Compile(body + flags)
				if err != nil {
					regexCache.Store(pattern, nil)
					return nil, err
				}
				regexCache.Store(pattern, re)
				return re, nil
			}
		}
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		regexCache.Store(pattern, nil)
		return nil, err
	}
	regexCache.Store(pattern, re)
	return re, nil
}

func IsPrivateIP(address string, patterns []string) bool {
	if v, ok := ipCache.Load(address); ok {
		return v.(bool)
	}
	priv := false
	for _, p := range patterns {
		re, err := CompileRegex(p)
		if err != nil || re == nil {
			continue
		}
		if re.MatchString(address) {
			priv = true
			break
		}
	}
	ipCache.Store(address, priv)
	return priv
}

func ClearIPCache(address string) { ipCache.Delete(address) }

type LoginState struct {
	mu       sync.Mutex
	attempts map[string]*loginEntry
}

type loginEntry struct {
	count, dailyCount        int
	lastAttempt, lockoutUntil time.Time
	dayStart                 time.Time
}

func NewLoginState() *LoginState {
	return &LoginState{attempts: make(map[string]*loginEntry)}
}

func (l *LoginState) LockoutUntil(ip string) time.Time {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.attempts[ip]
	if !ok {
		return time.Time{}
	}
	if e.lockoutUntil.After(time.Now()) {
		return e.lockoutUntil
	}
	return time.Time{}
}

func (l *LoginState) ProcessLogin(ip, hashedInput, adminPassword string) (bool, string, time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	e := l.attempts[ip]
	if e == nil {
		e = &loginEntry{dayStart: now}
		l.attempts[ip] = e
	}
	if now.Sub(e.dayStart) > 24*time.Hour {
		e.dailyCount = 0
		e.dayStart = now
	}
	if e.lockoutUntil.After(now) {
		return false, "Too many failed attempts. Access restricted.", e.lockoutUntil
	}
	if hashedInput == adminPassword && adminPassword != "" {
		delete(l.attempts, ip)
		return true, "Login successful.", time.Time{}
	}
	e.count++
	e.dailyCount++
	e.lastAttempt = now
	if e.dailyCount >= 40 {
		e.lockoutUntil = now.Add(7 * 24 * time.Hour)
	} else if e.dailyCount >= 20 {
		e.lockoutUntil = now.Add(24 * time.Hour)
	} else if e.count >= 10 {
		e.lockoutUntil = now.Add(time.Hour)
		e.count = 0
	}
	return false, "Authentication failed. Please check your credentials.", e.lockoutUntil
}

func RemoteAddr(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := splitComma(xff)
		if len(parts) > 0 {
			return parts[0]
		}
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if host == "" {
		return r.RemoteAddr
	}
	return host
}

func RemoteAddrWithPort(r *http.Request) string {
	ip := RemoteAddr(r)
	_, port, _ := net.SplitHostPort(r.RemoteAddr)
	if port != "" {
		return ip + ":" + port
	}
	return ip
}

func splitComma(s string) []string {
	var out []string
	for _, p := range stringsSplit(s, ',') {
		p = trim(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func stringsSplit(s string, sep byte) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func trim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

func PortAvailable(host string, port int) bool {
	addr := host
	if addr == "" {
		addr = "0.0.0.0"
	}
	if strings.Contains(addr, ":") && !strings.HasPrefix(addr, "[") {
		addr = fmt.Sprintf("[%s]:%d", addr, port)
	} else {
		addr = fmt.Sprintf("%s:%d", addr, port)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	ln.Close()
	return true
}
