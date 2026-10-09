package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestSignMessage(t *testing.T) {
	// RFC 4231, test case 1: independently specified HMAC-SHA256 vector.
	want := []byte{0xb0, 0x34, 0x4c, 0x61, 0xd8, 0xdb, 0x38, 0x53, 0x5c, 0xa8, 0xaf, 0xce, 0xaf, 0x0b, 0xf1, 0x2b, 0x88, 0x1d, 0xc2, 0x00, 0xc9, 0x83, 0x3d, 0xa7, 0x26, 0xe9, 0x37, 0x6c, 0x2e, 0x32, 0xcf, 0xf7}
	encoded := signMessage(bytes.Repeat([]byte{0x0b}, 20), []byte("Hi There"))
	got, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("digest=%x; want=%x", got, want)
	}
	if encoded == signMessage([]byte("different key"), []byte("Hi There")) || encoded == signMessage(bytes.Repeat([]byte{0x0b}, 20), []byte("modified")) {
		t.Fatal("signature did not bind key and payload")
	}
}

func TestDeliveryHonorsPreCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := testApplication().performDelivery(nil, ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled delivery error=%v", err)
	}
	done := make(chan struct{})
	go func() { testApplication().runDeliveryWorker(ctx); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker failed to honor pre-canceled context")
	}
}

func TestExponentialBackoffScheduler(t *testing.T) {
	// Full jitter permits zero delay. Assert bounds, never a random exact delay.
	for attempt := int64(0); attempt < 5; attempt++ {
		t.Run(fmt.Sprintf("attempt_%d", attempt+1), func(t *testing.T) {
			upper := 5 * time.Minute * time.Duration(1<<uint(attempt))
			for i := 0; i < 25; i++ {
				before := time.Now()
				next := exponentialBackoffScheduler(attempt)
				after := time.Now()
				if next.Before(before) || !next.Before(after.Add(upper)) {
					t.Fatalf("next=%s outside [%s,%s)", next, before, after.Add(upper))
				}
			}
		})
	}
}
