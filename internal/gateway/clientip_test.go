package gateway

import (
	"net/http"
	"net/netip"
	"testing"
)

func TestClientIP(t *testing.T) {
	proxies := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("127.0.0.1/32")}
	cases := []struct {
		name    string
		remote  string
		xff     []string
		trusted []netip.Prefix
		want    string
	}{
		{"direct", "203.0.113.7:5000", nil, proxies, "203.0.113.7"},
		{"untrusted peer ignores header", "203.0.113.7:5000", []string{"1.2.3.4"}, proxies, "203.0.113.7"},
		{"no proxies configured", "10.0.0.2:5000", []string{"1.2.3.4"}, nil, "10.0.0.2"},
		{"one proxy", "10.0.0.2:5000", []string{"198.51.100.9"}, proxies, "198.51.100.9"},
		{"spoofed left entry is ignored", "10.0.0.2:5000", []string{"6.6.6.6, 198.51.100.9"}, proxies, "198.51.100.9"},
		{"proxy chain", "127.0.0.1:5000", []string{"198.51.100.9, 10.1.1.1"}, proxies, "198.51.100.9"},
		{"several headers", "10.0.0.2:5000", []string{"6.6.6.6", "198.51.100.9"}, proxies, "198.51.100.9"},
		{"trusted peer without header", "10.0.0.2:5000", nil, proxies, "10.0.0.2"},
		{"malformed hop", "10.0.0.2:5000", []string{"198.51.100.9, junk"}, proxies, "10.0.0.2"},
		{"only proxies", "10.0.0.2:5000", []string{"10.3.3.3"}, proxies, "10.3.3.3"},
		{"ipv6 peer", "[2001:db8::1]:5000", nil, proxies, "2001:db8::1"},
		{"ipv4-mapped peer", "[::ffff:10.0.0.2]:5000", []string{"198.51.100.9"}, proxies, "198.51.100.9"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &http.Request{RemoteAddr: tc.remote, Header: http.Header{}}
			for _, v := range tc.xff {
				r.Header.Add("X-Forwarded-For", v)
			}
			if got := clientIP(r, tc.trusted); got != tc.want {
				t.Fatalf("clientIP = %q, want %q", got, tc.want)
			}
		})
	}
}
