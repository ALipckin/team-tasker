package middleware

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// RealIP replaces RemoteAddr with the client address from X-Forwarded-For when
// the request came through a trusted proxy. It selects the rightmost address
// that is not a trusted proxy, since clients can spoof addresses on the left.
func RealIP(trusted []*net.IPNet) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if len(trusted) == 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isTrusted(trusted, ClientIP(r)) {
				if ip := forwardedFor(trusted, r.Header.Values("X-Forwarded-For")); ip != "" {
					r.RemoteAddr = net.JoinHostPort(ip, "0")
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func forwardedFor(trusted []*net.IPNet, headers []string) string {
	var hops []string
	for _, header := range headers {
		hops = append(hops, strings.Split(header, ",")...)
	}
	for i := len(hops) - 1; i >= 0; i-- {
		hop := parseHop(hops[i])
		if hop == "" {
			return ""
		}
		if !isTrusted(trusted, hop) {
			return hop
		}
	}
	return ""
}

func parseHop(hop string) string {
	hop = strings.TrimSpace(hop)
	if host, _, err := net.SplitHostPort(hop); err == nil {
		hop = host
	}
	hop = strings.TrimSuffix(strings.TrimPrefix(hop, "["), "]")
	addr, err := netip.ParseAddr(hop)
	if err != nil {
		return ""
	}
	return addr.WithZone("").Unmap().String()
}

func isTrusted(trusted []*net.IPNet, addr string) bool {
	ip := net.ParseIP(addr)
	if ip == nil {
		return false
	}
	for _, network := range trusted {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
