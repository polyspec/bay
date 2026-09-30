package stack

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestServiceNameWaitsForDeclaredAddresses(t *testing.T) {
	in := Instance{Name: "app-web", IPv4: "192.0.2.10", IPv6: "fd00::10"}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	calls := 0
	var progress []string
	err := waitServiceName(ctx, in, func(ctx context.Context, network, host string) ([]netip.Addr, error) {
		calls++
		if network != "ip" || host != "app-web."+BackendDomain {
			t.Fatalf("lookup %s %s", network, host)
		}
		if calls == 1 {
			return []netip.Addr{netip.MustParseAddr("192.0.2.9"), netip.MustParseAddr("fd00::9")}, nil
		}
		return []netip.Addr{netip.MustParseAddr("fd00::10"), netip.MustParseAddr("192.0.2.10")}, nil
	}, func(line string) { progress = append(progress, line) })
	if err != nil || calls != 2 {
		t.Fatalf("hostname readiness calls=%d error=%v", calls, err)
	}
	if len(progress) != 3 || !strings.Contains(progress[1], "resolved") || !strings.Contains(progress[2], "ready after") {
		t.Fatalf("incomplete progress: %v", progress)
	}
}

func TestServiceNameDoesNotAcceptIncompleteOrObsoleteAddresses(t *testing.T) {
	for _, addresses := range [][]netip.Addr{
		nil,
		{netip.MustParseAddr("192.0.2.10")},
		{netip.MustParseAddr("192.0.2.10"), netip.MustParseAddr("fd00::9")},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
		err := waitServiceName(ctx, Instance{Name: "app-web", IPv4: "192.0.2.10", IPv6: "fd00::10"},
			func(context.Context, string, string) ([]netip.Addr, error) { return addresses, nil }, nil)
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "last lookup: resolved") {
			t.Fatalf("obsolete or incomplete addresses were accepted: %v", err)
		}
	}
}

func TestServiceNameReportsLookupFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	err := waitServiceName(ctx, Instance{Name: "app-web", IPv4: "192.0.2.10"},
		func(context.Context, string, string) ([]netip.Addr, error) { return nil, errors.New("DNS unavailable") }, nil)
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "DNS unavailable") {
		t.Fatalf("lookup failure was not reported: %v", err)
	}
}

func TestServiceNameRejectsInvalidDeclarationBeforeLookup(t *testing.T) {
	for _, address := range []string{"", "invalid"} {
		err := waitServiceName(context.Background(), Instance{Name: "app-web", IPv4: address},
			func(context.Context, string, string) ([]netip.Addr, error) {
				t.Fatal("invalid declaration reached DNS")
				return nil, nil
			}, nil)
		if err == nil {
			t.Fatal("invalid declaration accepted")
		}
	}
}

func TestServiceNameFailureStopsDependants(t *testing.T) {
	cfg := lifecycleConfig(t)
	e, f := fakeEngine(t)
	e.timeout = 20 * time.Millisecond
	e.resolveName = func(context.Context, string, string) ([]netip.Addr, error) {
		return nil, errors.New("DNS still reports removed service")
	}
	err := e.start(cfg, cfg.Sorted(), false)
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "DNS still reports removed service") {
		t.Fatalf("hostname failure did not stop dependent startup: %v", err)
	}
	for _, name := range []string{"app-initialize", "app-web"} {
		if _, exists := f.instances[name]; exists {
			t.Fatalf("dependent service started before DNS readiness: %s", name)
		}
	}
}
