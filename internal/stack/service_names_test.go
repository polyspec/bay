package stack

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"
)

func eventFixture(events []nameEvent, closed bool, cleanup func() error) *nameSubscription {
	stream := make(chan nameEvent, len(events))
	for _, event := range events {
		stream <- event
	}
	if closed {
		close(stream)
	}
	return &nameSubscription{events: stream, close: cleanup}
}

func TestServiceNameWaitsForDeclaredAddresses(t *testing.T) {
	old4, old6 := netip.MustParseAddr("192.0.2.9"), netip.MustParseAddr("fd00::9")
	new4, new6 := netip.MustParseAddr("192.0.2.10"), netip.MustParseAddr("fd00::10")
	in := Instance{Name: "app-web", IPv4: new4.String(), IPv6: new6.String()}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	calls, closes := 0, 0
	var progress []string
	err := waitServiceName(ctx, in, func(ctx context.Context, host string, expected []netip.Addr) (*nameSubscription, error) {
		calls++
		if host != "app-web."+BackendDomain || !slices.Equal(expected, []netip.Addr{new4, new6}) {
			t.Fatalf("subscription %s %v", host, expected)
		}
		return eventFixture([]nameEvent{
			{address: old4, added: true}, {address: old6, added: true},
			{address: old4}, {address: new4, added: true},
			{address: old6}, {address: new6, added: true},
		}, true, func() error { closes++; return nil }), nil
	}, func(line string) { progress = append(progress, line) })
	if err != nil || calls != 1 || closes != 1 {
		t.Fatalf("readiness subscriptions=%d cleanup=%d error=%v", calls, closes, err)
	}
	if len(progress) != 7 {
		t.Fatalf("incomplete event progress: %v", progress)
	}
}

func TestServiceNameDoesNotAcceptIncompleteOrObsoleteAddresses(t *testing.T) {
	for _, events := range [][]nameEvent{
		nil,
		{{address: netip.MustParseAddr("192.0.2.10"), added: true}},
		{{address: netip.MustParseAddr("192.0.2.10"), added: true}, {address: netip.MustParseAddr("fd00::9"), added: true}},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
		closes := 0
		err := waitServiceName(ctx, Instance{Name: "app-web", IPv4: "192.0.2.10", IPv6: "fd00::10"},
			func(context.Context, string, []netip.Addr) (*nameSubscription, error) {
				return eventFixture(events, false, func() error { closes++; return nil }), nil
			}, nil)
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) || closes != 1 {
			t.Fatalf("incomplete event set accepted or not collected: %v closes=%d", err, closes)
		}
	}
}

func TestServiceNameReportsEventAndCleanupFailures(t *testing.T) {
	want, cleanup := errors.New("DNS event failed"), errors.New("DNS close failed")
	err := waitServiceName(context.Background(), Instance{Name: "app-web", IPv4: "192.0.2.10"},
		func(context.Context, string, []netip.Addr) (*nameSubscription, error) {
			return eventFixture([]nameEvent{{err: want}}, true, func() error { return cleanup }), nil
		}, nil)
	if !errors.Is(err, want) || !errors.Is(err, cleanup) {
		t.Fatalf("event or cleanup error lost: %v", err)
	}
}

func TestServiceNameRejectsInvalidDeclarationBeforeSubscription(t *testing.T) {
	for _, address := range []string{"", "invalid"} {
		err := waitServiceName(context.Background(), Instance{Name: "app-web", IPv4: address},
			func(context.Context, string, []netip.Addr) (*nameSubscription, error) {
				t.Fatal("invalid declaration reached DNS")
				return nil, nil
			}, nil)
		if err == nil {
			t.Fatal("invalid declaration accepted")
		}
	}
}

func TestServiceNameRejectsStoppedSubscriptionAndInvalidEvent(t *testing.T) {
	for _, events := range [][]nameEvent{nil, {{added: true}}} {
		closes := 0
		err := waitServiceName(context.Background(), Instance{Name: "app-web", IPv4: "192.0.2.10"},
			func(context.Context, string, []netip.Addr) (*nameSubscription, error) {
				return eventFixture(events, true, func() error { closes++; return nil }), nil
			}, nil)
		if err == nil || closes != 1 {
			t.Fatalf("invalid stream accepted: %v closes=%d", err, closes)
		}
	}
}

func TestServiceNameChecksCompleteEventBatch(t *testing.T) {
	address := netip.MustParseAddr("192.0.2.10")
	err := waitServiceName(context.Background(), Instance{Name: "app-web", IPv4: address.String()},
		func(context.Context, string, []netip.Addr) (*nameSubscription, error) {
			return eventFixture([]nameEvent{{address: address, added: true, more: true}, {address: address}}, true, func() error { return nil }), nil
		}, nil)
	if err == nil {
		t.Fatal("readiness was accepted before the event batch finished")
	}
}

func TestServiceNameFailureStopsDependants(t *testing.T) {
	cfg := lifecycleConfig(t)
	e, f := fakeEngine(t)
	want := errors.New("DNS still reports removed service")
	e.subscribeName = func(context.Context, string, []netip.Addr) (*nameSubscription, error) { return nil, want }
	err := e.start(cfg, cfg.Sorted(), false)
	if !errors.Is(err, want) || !strings.Contains(err.Error(), "DNS still reports removed service") {
		t.Fatalf("hostname failure did not stop dependent startup: %v", err)
	}
	for _, name := range []string{"app-initialize", "app-web"} {
		if _, exists := f.instances[name]; exists {
			t.Fatalf("dependent started before DNS readiness: %s", name)
		}
	}
}
