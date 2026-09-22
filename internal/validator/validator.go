package validator

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"time"
)

type Validator struct {
	Errors map[string]string
}

func New() *Validator {
	return &Validator{
		Errors: make(map[string]string),
	}
}

func (v *Validator) Valid() bool {
	return len(v.Errors) == 0
}

func (v *Validator) AddError(key, value string) {
	if _, exists := v.Errors[key]; !exists {
		v.Errors[key] = value
	}
}

func (v *Validator) Check(ok bool, key, value string) {
	if !ok {
		v.AddError(key, value)
	}
}

func ValidURL(callback_url string) bool {
	urlBody, err := url.ParseRequestURI(callback_url)
	if err != nil {

		return false
	}

	if urlBody.Scheme != "http" && urlBody.Scheme != "https" {
		return false
	}
	if urlBody.Hostname() == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	addresses, err := net.DefaultResolver.LookupHost(ctx, urlBody.Hostname())
	if err != nil {
		return false
	}
	for _, addr := range addresses {
		netip_addr, err := netip.ParseAddr(addr)
		if err != nil {
			return false
		}
		if netip_addr.IsLoopback() ||
			netip_addr.IsPrivate() ||
			netip_addr.IsLinkLocalUnicast() ||
			netip_addr.IsLinkLocalMulticast() ||
			netip_addr.IsMulticast() ||
			netip_addr.IsUnspecified() {
			return false
		}
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Head(callback_url)
	if err != nil {
		return false
	}
	return resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices

}

func Unique[T comparable](values []T) bool {
	unique := make(map[T]bool)
	for _, value := range values {
		unique[value] = true
	}
	return len(values) == len(unique)
}
