//go:build darwin && cgo

package stack

import (
	"context"
	"net/netip"
	"testing"
	"time"
)

func TestNativeNameSubscriptionCloseCancelsSocketWait(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	subscription, err := nativeNameSubscription(ctx, "unassigned-fixture.container.test", []netip.Addr{netip.MustParseAddr("192.0.2.1")})
	if err != nil {
		t.Fatal(err)
	}
	collected := make(chan error, 1)
	go func() { collected <- subscription.close() }()
	select {
	case err := <-collected:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("native subscription did not collect socket cancellation")
	}
	if err := subscription.close(); err != nil {
		t.Fatalf("repeated close: %v", err)
	}
	if _, open := <-subscription.events; open {
		// A callback already received may be buffered; cancellation must still close
		// the channel after those received events are consumed.
		for range subscription.events {
		}
	}
}
