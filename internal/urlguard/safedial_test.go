package urlguard

import (
	"errors"
	"net/netip"
	"testing"
)

func TestIsPublicIP(t *testing.T) {
	if IsPublicIP(netip.Addr{}) {
		t.Fatal("invalid address accepted")
	}
	for _, address := range []string{
		"0.0.0.0", "0.1.2.3", "10.0.0.1", "127.0.0.1", "172.16.0.1", "172.31.255.255",
		"192.168.1.1", "169.254.1.1", "100.64.0.1", "100.127.255.255", "192.0.0.1",
		"192.0.2.1", "198.18.0.1", "198.19.255.255", "198.51.100.1", "203.0.113.1",
		"224.0.0.1", "240.0.0.1", "255.255.255.255", "::", "::1", "::ffff:10.0.0.1",
		"64:ff9b::808:808", "100::1", "2001:db8::1", "fc00::1", "fdff::1", "fe80::1%eth0", "ff02::1",
	} {
		t.Run(address, func(t *testing.T) {
			if IsPublicIP(netip.MustParseAddr(address)) {
				t.Fatalf("blocked address %s accepted", address)
			}
		})
	}
	for _, address := range []string{"8.8.8.8", "1.1.1.1", "100.63.255.255", "100.128.0.0", "172.15.255.255", "172.32.0.0", "2606:4700:4700::1111", "::ffff:8.8.8.8"} {
		t.Run(address, func(t *testing.T) {
			if !IsPublicIP(netip.MustParseAddr(address)) {
				t.Fatalf("public address %s rejected", address)
			}
		})
	}
}

func TestControl(t *testing.T) {
	for _, tc := range []struct {
		name, network, address string
		blocked                bool
		invalid                bool
	}{
		{"public ipv4", "tcp4", "8.8.8.8:443", false, false},
		{"public ipv6", "tcp6", "[2606:4700:4700::1111]:443", false, false},
		{"loopback", "tcp", "127.0.0.1:80", true, false},
		{"resolved private", "tcp", "10.0.0.1:80", true, false},
		{"mapped loopback", "tcp6", "[::ffff:127.0.0.1]:80", true, false},
		{"unresolved hostname", "tcp", "example.com:80", true, false},
		{"unix", "unix", "/tmp/socket", true, false},
		{"udp", "udp", "8.8.8.8:53", true, false},
		{"malformed", "tcp", "missing-port", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := control(tc.network, tc.address, nil)
			if tc.blocked {
				if !errors.Is(err, ErrBlockedDestination) {
					t.Fatalf("got %v; want blocked destination", err)
				}
				return
			}
			if tc.invalid {
				if err == nil {
					t.Fatal("malformed address accepted")
				}
				return
			}
			if err != nil {
				t.Fatalf("public destination rejected: %v", err)
			}
		})
	}
}
