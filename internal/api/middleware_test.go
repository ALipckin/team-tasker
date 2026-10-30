package api

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/internal/api/middleware"
)

func TestRealIP(t *testing.T) {
	_, proxies, _ := net.ParseCIDR("10.0.0.0/8")
	trusted := []*net.IPNet{proxies}

	tests := []struct {
		name   string
		remote string
		xff    string
		want   string
	}{
		{"direct client ignores header", "203.0.113.5:1234", "1.1.1.1", "203.0.113.5"},
		{"trusted proxy", "10.0.0.2:1234", "198.51.100.7", "198.51.100.7"},
		{"spoofed left part is ignored", "10.0.0.2:1234", "1.1.1.1, 198.51.100.7, 10.0.0.3", "198.51.100.7"},
		{"garbage header keeps proxy address", "10.0.0.2:1234", "not-an-ip", "10.0.0.2"},
		{"hop with port", "10.0.0.2:1234", "198.51.100.7:51234", "198.51.100.7"},
		{"bracketed ipv6", "10.0.0.2:1234", "[2001:db8::1]", "2001:db8::1"},
		{"bracketed ipv6 with port", "10.0.0.2:1234", "[2001:db8::1]:51234", "2001:db8::1"},
		{"zoned ipv6", "10.0.0.2:1234", "fe80::1%eth0", "fe80::1"},
		{"ipv4-mapped ipv6", "10.0.0.2:1234", "::ffff:198.51.100.7", "198.51.100.7"},
		{"trusted hop with port is skipped", "10.0.0.2:1234", "198.51.100.7, 10.0.0.3:443", "198.51.100.7"},
		{"unknown keeps proxy address", "10.0.0.2:1234", "unknown", "10.0.0.2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			h := middleware.RealIP(trusted)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got = middleware.ClientIP(r)
			}))

			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tt.remote
			r.Header.Set("X-Forwarded-For", tt.xff)
			h.ServeHTTP(httptest.NewRecorder(), r)

			if got != tt.want {
				t.Errorf("client ip = %q, want %q", got, tt.want)
			}
		})
	}
}
