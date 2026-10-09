//go:build integration

package urlguard

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"testing"
	"time"
)

func TestIntegrationDialerRejectsPrivateDNSAnswer(t *testing.T) {
	// A local DNS server resolves an otherwise acceptable hostname to loopback.
	// No external DNS or callback endpoint is contacted.
	dns, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 1500)
		for {
			n, peer, err := dns.ReadFrom(buf)
			if err != nil {
				return
			}
			if n < 12 {
				continue
			}
			end := 12
			for end < n && buf[end] != 0 {
				end += int(buf[end]) + 1
			}
			end++ // terminating label
			if end+4 > n {
				continue
			}
			qtype := binary.BigEndian.Uint16(buf[end : end+2])
			response := append([]byte(nil), buf[:end+4]...)
			binary.BigEndian.PutUint16(response[2:4], 0x8180) // successful response
			binary.BigEndian.PutUint16(response[6:8], 0)
			binary.BigEndian.PutUint16(response[8:10], 0)
			binary.BigEndian.PutUint16(response[10:12], 0)
			if qtype == 1 { // A record: 127.0.0.1
				binary.BigEndian.PutUint16(response[6:8], 1)
				response = append(response, 0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 1, 0, 4, 127, 0, 0, 1)
			}
			if _, err := dns.WriteTo(response, peer); err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		dns.Close()
		<-done
	})
	host := "callback.example.test"
	if !ValidURL("http://" + host + "/hook") {
		t.Fatal("public hostname rejected before resolution")
	}
	dialer := Dialer(time.Second)
	dialer.Resolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "udp", dns.LocalAddr().String())
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, "80"))
	if conn != nil {
		conn.Close()
		t.Fatal("dialer connected to a private DNS destination")
	}
	if !errors.Is(err, ErrBlockedDestination) {
		t.Fatalf("dial error = %v; want ErrBlockedDestination", err)
	}
}
