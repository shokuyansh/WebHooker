package urlguard

import (
	"testing"
)

func TestValidURL(t *testing.T) {
	for _, tc := range []struct {
		url  string
		want bool
	}{
		{"https://example.com/callback?token=abc", true}, {"http://example.com:8080/hook", true},
		{"https://8.8.8.8/hook", true}, {"https://[2606:4700:4700::1111]/hook", true},
		{"http://example.com:1", true}, {"http://example.com:65535", true},
		{"", false}, {"/relative", false}, {"ftp://example.com", false}, {"https:///hook", false},
		{"https://user:pass@example.com", false}, {"http://localhost", false},
		{"http://LOCALHOST./hook", false}, {"http://service.localhost", false},
		{"http://metadata.google.internal", false}, {"http://metadata", false},
		{"http://instance-data", false}, {"http://service.local", false},
		{"http://service.internal", false}, {"http://host.home.arpa", false},
		{"http://127.0.0.1", false}, {"http://10.1.2.3", false}, {"http://169.254.169.254", false},
		{"http://[::1]", false}, {"http://[::ffff:127.0.0.1]", false},
		{"http://192.0.2.1", false}, {"http://[2001:db8::1]", false},
		{"http://example.com:0", false}, {"http://example.com:65536", false},
		{"http://example.com:bad", false}, {"http://example.com/%zz", false},
	} {
		t.Run(tc.url, func(t *testing.T) {
			if got := ValidURL(tc.url); got != tc.want {
				t.Fatalf("ValidURL(%q) = %v; want %v", tc.url, got, tc.want)
			}
		})
	}
}
