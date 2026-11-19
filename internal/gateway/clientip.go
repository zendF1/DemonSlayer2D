package gateway

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// clientIP is the address rate limits are keyed on. A direct connection uses its
// peer address. When the peer is a trusted proxy, X-Forwarded-For is walked from
// the right (the end each proxy appends to) and the first address that is not a
// trusted proxy is the player. Entries further left were written by the client and
// are never believed.
func clientIP(r *http.Request, trusted []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	peer = peer.Unmap()
	if !isTrusted(peer, trusted) {
		return peer.String()
	}
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	ip := peer
	for i := len(hops) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break // malformed: stop at the last hop we could read
		}
		ip = a.Unmap()
		if !isTrusted(ip, trusted) {
			break
		}
	}
	return ip.String()
}

func isTrusted(ip netip.Addr, trusted []netip.Prefix) bool {
	for _, p := range trusted {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}
