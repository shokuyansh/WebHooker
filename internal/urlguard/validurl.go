// Package urlguard validates and constrains the use of user-supplied callback
// URLs.
//
// It separates two questions that must not be confused:
//
//   - ValidURL answers "is this URL syntactically well formed and does it
//     obviously point at internal infrastructure?" It is offline and safe to
//     call while a webhook is being registered.
//   - The dialer in safedial.go answers "may this connection actually be made
//     to the address it resolved to?" It is enforced at connect time, because
//     a check performed before delivery can be invalidated by DNS rebinding
//     in the window between validation and the connection attempt.
//
// Neither function establishes that a callback URL is reachable. Reachability
// is a property of the endpoint at a moment in time, not of the URL, and
// attempting an HTTP request against an arbitrary third-party URL merely to
// find out is a probe the registering user did not ask for.
package urlguard

import (
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

// blockedHostNames are exact hostnames that must never be accepted as a
// callback destination.
var blockedHostNames = map[string]bool{
	"localhost":                true,
	"metadata":                 true,
	"metadata.google.internal": true,
	"instance-data":            true,
}

// blockedHostSuffixes are hostname suffixes that are conventionally used for
// names resolvable only inside a local or private network.
var blockedHostSuffixes = []string{
	".localhost",
	".local",
	".internal",
	".home.arpa",
}

// ValidURL reports whether callback_url is acceptable as a webhook callback
// URL. It performs no network requests.
//
// Checks performed:
//
//   - the URL parses and is absolute
//   - the scheme is http or https
//   - no embedded credentials, which would leak into logs and error messages
//   - a literal IP address is public (see IsPublicIP)
//   - a hostname does not match the internal-name denylist
//   - an explicit port, if present, is in range
//
// Passing this check is necessary but not sufficient for a safe delivery; the
// Dialer enforces the same policy against the resolved address at connect time.
func ValidURL(callback_url string) bool {
	u, err := url.ParseRequestURI(callback_url)
	if err != nil {
		return false
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}

	if u.User != nil {
		return false
	}

	host := u.Hostname()
	if host == "" {
		return false
	}

	if isBlockedHostname(host) {
		return false
	}

	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return false
		}
	}

	if addr, err := netip.ParseAddr(host); err == nil {
		// A literal address needs no lookup; judge it directly.
		return IsPublicIP(addr)
	}

	return true
}

// isBlockedHostname reports whether host is one of the hostnames that resolve
// only within a local or private network, or that are reserved by convention
// for instance metadata services.
func isBlockedHostname(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if blockedHostNames[host] {
		return true
	}
	for _, suffix := range blockedHostSuffixes {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}
