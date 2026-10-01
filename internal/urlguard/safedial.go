// Package urlguard provides the primitives required to reach untrusted,
// user-supplied callback URLs without allowing access to internal network
// resources. See validurl.go for the registration-time check that pairs with
// the dialer below.
//
// SSRF protection at delivery time cannot be implemented by resolving the
// hostname and inspecting the result before the HTTP client runs: that
// creates a time-of-check/time-of-use gap which DNS rebinding can exploit
// between the lookup and the connect.
//
// net.Dialer.Control runs on the connection after name resolution and
// immediately before the socket is connected, so the address validated is
// provably the address that is reached.
package urlguard

import (
	"errors"
	"net"
	"net/netip"
	"syscall"
	"time"
)

// ErrBlockedDestination is returned when a resolved address belongs to a
// network range that must never be reached by a webhook delivery.
var ErrBlockedDestination = errors.New("urlguard: destination address is not routable")

// blockedNamespaces are the address ranges that callback URLs must never
// resolve to.
var blockedNamespaces = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),       // current network
	netip.MustParsePrefix("100.64.0.0/10"),   // carrier-grade NAT
	netip.MustParsePrefix("192.0.0.0/24"),    // IETF protocol assignments
	netip.MustParsePrefix("192.0.2.0/24"),    // TEST-NET-1
	netip.MustParsePrefix("198.18.0.0/15"),   // benchmarking
	netip.MustParsePrefix("198.51.100.0/24"), // TEST-NET-2
	netip.MustParsePrefix("203.0.113.0/24"),  // TEST-NET-3
	netip.MustParsePrefix("240.0.0.0/4"),     // reserved
	netip.MustParsePrefix("::/128"),          // unspecified
	netip.MustParsePrefix("64:ff9b::/96"),    // NAT64
	netip.MustParsePrefix("100::/64"),        // discard-only
	netip.MustParsePrefix("2001:db8::/32"),   // documentation
	netip.MustParsePrefix("fc00::/7"),        // unique local
	netip.MustParsePrefix("fe80::/10"),       // link-local
	netip.MustParsePrefix("ff00::/8"),        // multicast
}

// IsPublicIP reports whether addr is safe to use as a webhook delivery
// destination. IPv4-mapped IPv6 addresses are unmapped before checking so
// that ::ffff:127.0.0.1 is rejected by the same rule as 127.0.0.1.
func IsPublicIP(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	addr = addr.Unmap().WithZone("")

	if addr.IsLoopback() ||
		addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsInterfaceLocalMulticast() ||
		addr.IsMulticast() ||
		addr.IsUnspecified() {
		return false
	}

	for _, prefix := range blockedNamespaces {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

// Dialer returns a net.Dialer that refuses to connect to addresses outside
// the public internet. Use its DialContext method as the DialContext field of
// an http.Transport.
func Dialer(timeout time.Duration) *net.Dialer {
	return &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
		Control:   control,
	}
}

// control validates the already-resolved destination address just before the
// connection is established.
//
// The switch is an allowlist, not a rejection list: the listed networks are
// permitted and fall through with an empty case, and only networks absent from
// the list are blocked. net/http hardcodes "tcp" at every connection attempt
// (see net/http.Transport.dialConn), so for an HTTP transport this never
// fires. It exists so that everything below can rely on address really being
// "host:port" with an IP in it, and to keep the dialer safe if it is ever
// reused for a non-HTTP call -- notably "unix", where address is a filesystem
// path such as /var/run/docker.sock and there is no IP to inspect at all.
func control(network, address string, _ syscall.RawConn) error {
	switch network {
	case "tcp", "tcp4", "tcp6":
	default:
		return ErrBlockedDestination
	}

	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return ErrBlockedDestination
	}
	if !IsPublicIP(addr) {
		return ErrBlockedDestination
	}
	return nil
}
