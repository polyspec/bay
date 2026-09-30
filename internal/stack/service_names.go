package stack

import (
	"context"
	"fmt"
	"net/netip"
	"slices"
	"time"
)

const serviceNameTimeout = 60 * time.Second

func (e *serviceEngine) waitServiceName(name string) error {
	if e.resolveName == nil {
		return fmt.Errorf("%s has no hostname resolver", name)
	}
	ctx, cancel := context.WithTimeout(context.Background(), min(serviceNameTimeout, e.timeout))
	defer cancel()
	in, found, err := e.lookup(ctx, name)
	if err != nil {
		return err
	}
	if !found || in.State != "running" {
		return fmt.Errorf("%s is unavailable for hostname readiness", name)
	}
	return waitServiceName(ctx, in, e.resolveName, e.progress)
}

func waitServiceName(ctx context.Context, in Instance,
	resolve func(context.Context, string, string) ([]netip.Addr, error), say func(string),
) error {
	host := in.Name + "." + BackendDomain
	expected := make([]netip.Addr, 0, 2)
	for _, value := range []string{in.IPv4, in.IPv6} {
		if value == "" {
			continue
		}
		address, err := netip.ParseAddr(value)
		if err != nil {
			return fmt.Errorf("%s has an invalid declared address: %w", host, err)
		}
		expected = append(expected, address.Unmap())
	}
	if len(expected) == 0 {
		return fmt.Errorf("%s has no declared address", host)
	}
	slices.SortFunc(expected, netip.Addr.Compare)
	expected = slices.Compact(expected)
	started := time.Now()
	if say != nil {
		say(fmt.Sprintf("%s: waiting for hostname addresses %v", host, expected))
	}
	var last error
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("%s hostname readiness failed: %w; last lookup: %v", host, err, last)
		}
		actual, err := resolve(ctx, "ip", host)
		if failure := ctx.Err(); failure != nil {
			return fmt.Errorf("%s hostname readiness failed: %w; last lookup: %v", host, failure, err)
		}
		if err == nil {
			for i, address := range actual {
				actual[i] = address.Unmap()
			}
			slices.SortFunc(actual, netip.Addr.Compare)
			actual = slices.Compact(actual)
			if slices.Equal(actual, expected) {
				if say != nil {
					say(fmt.Sprintf("%s: hostname addresses ready after %s", host, time.Since(started).Round(time.Millisecond)))
				}
				return nil
			}
			err = fmt.Errorf("resolved %v, expected %v", actual, expected)
		}
		last = err
		if say != nil {
			say(fmt.Sprintf("%s: hostname not ready after %s: %v", host, time.Since(started).Round(time.Millisecond), last))
		}
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("%s hostname readiness failed: %w; last lookup: %v", host, ctx.Err(), last)
		case <-timer.C:
		}
	}
}
