//go:build darwin && cgo

package stack

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"slices"
	"testing"
	"time"
)

// negativeNameReadiness bounds the readiness wait in this test. The system
// resolver keeps a negative answer without SOA for at least 60 seconds, so a
// subscription that is answered from that entry cannot finish within this
// bound.
const negativeNameReadiness = 20 * time.Second

// TestNativeNameSubscriptionReplacesCachedNegativeAnswer looks up a service
// name before its container exists, which makes the system resolver cache the
// runtime DNS's NXDOMAIN answer. Containers resolve through the network
// gateway, which answers from the same cache. Readiness must receive the
// container's addresses from the runtime DNS and replace that entry, so the
// system resolver returns the addresses afterwards.
func TestNativeNameSubscriptionReplacesCachedNegativeAnswer(t *testing.T) {
	if os.Getenv("BAY_E2E") == "" {
		t.Skip("set BAY_E2E=1 to run tests that start containers")
	}
	name := fmt.Sprintf("bay-negative-%d", time.Now().UnixNano())
	host := name + "." + BackendDomain
	t.Cleanup(func() { Remove(name) })

	lookup := func() ([]netip.Addr, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	}
	notFound := func(err error) bool {
		var dnsErr *net.DNSError
		return errors.As(err, &dnsErr) && dnsErr.IsNotFound
	}
	if addresses, err := lookup(); !notFound(err) {
		t.Fatalf("lookup of %s before creation = %v, %v; want a not-found answer", host, addresses, err)
	}

	out, err := exec.Command(engineBin(AppleEngine), "run", "--detach", "--name", name,
		"--network", ProxyNetwork, ProxyImage).CombinedOutput()
	if err != nil {
		t.Fatalf("starting %s: %v\n%s", name, err, out)
	}
	in := waitRunning(t, name)
	if addresses, err := lookup(); !notFound(err) {
		t.Fatalf("lookup of %s after creation = %v, %v; want the cached not-found answer", host, addresses, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), negativeNameReadiness)
	defer cancel()
	started := time.Now()
	if err := waitServiceName(ctx, in, nativeNameSubscription, func(line string) { t.Log(line) }); err != nil {
		t.Fatalf("readiness after a cached negative answer: %v", err)
	}
	t.Logf("%s ready after %s", host, time.Since(started).Round(time.Millisecond))

	want := []netip.Addr{netip.MustParseAddr(in.IPv4).Unmap()}
	if in.IPv6 != "" {
		want = append(want, netip.MustParseAddr(in.IPv6).Unmap())
	}
	slices.SortFunc(want, netip.Addr.Compare)
	got, err := lookup()
	if err != nil {
		t.Fatalf("lookup of %s after readiness: %v; want %v", host, err, want)
	}
	for i := range got {
		got[i] = got[i].Unmap()
	}
	slices.SortFunc(got, netip.Addr.Compare)
	got = slices.Compact(got)
	if !slices.Equal(got, want) {
		t.Fatalf("lookup of %s after readiness = %v, want %v", host, got, want)
	}
}
