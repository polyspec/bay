package stack

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"time"
)

const serviceNameTimeout = 60 * time.Second

type nameEvent struct {
	address netip.Addr
	added   bool
	more    bool
	err     error
}

type nameSubscription struct {
	events <-chan nameEvent
	close  func() error
}

type nameSubscriber func(context.Context, string, []netip.Addr) (*nameSubscription, error)

func (e *serviceEngine) waitServiceName(name string) error {
	if e.subscribeName == nil {
		return fmt.Errorf("%s has no hostname subscriber", name)
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
	return waitServiceName(ctx, in, e.subscribeName, e.progress)
}

func waitServiceName(ctx context.Context, in Instance, subscribe nameSubscriber, say func(string)) (result error) {
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
	if err := ctx.Err(); err != nil {
		return err
	}
	started := time.Now()
	if say != nil {
		deadline, bounded := ctx.Deadline()
		if bounded {
			say(fmt.Sprintf("%s: subscribing to hostname addresses %v, up to %s", host, expected, time.Until(deadline).Round(time.Millisecond)))
		} else {
			say(fmt.Sprintf("%s: subscribing to hostname addresses %v", host, expected))
		}
	}
	subscription, err := subscribe(ctx, host, expected)
	if err != nil {
		return err
	}
	if subscription == nil || subscription.close == nil {
		return errors.New("invalid hostname subscription")
	}
	defer func() {
		if err := subscription.close(); err != nil {
			result = errors.Join(result, err)
		}
	}()
	if subscription.events == nil {
		return errors.New("invalid hostname event channel")
	}
	addresses := make(map[netip.Addr]bool)
	var actual []netip.Addr
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s hostname readiness failed: %w; received %v, expected %v", host, ctx.Err(), actual, expected)
		case event, open := <-subscription.events:
			if err := ctx.Err(); err != nil {
				return err
			}
			if !open {
				return fmt.Errorf("%s hostname subscription stopped before readiness", host)
			}
			if event.err != nil {
				return fmt.Errorf("%s hostname event: %w", host, event.err)
			}
			if !event.address.IsValid() {
				return fmt.Errorf("%s hostname event has an invalid address", host)
			}
			address := event.address.Unmap()
			if event.added {
				addresses[address] = true
			} else {
				delete(addresses, address)
			}
			actual = actual[:0]
			for address := range addresses {
				actual = append(actual, address)
			}
			slices.SortFunc(actual, netip.Addr.Compare)
			if say != nil {
				say(fmt.Sprintf("%s: hostname addresses %v after %s", host, actual, time.Since(started).Round(time.Millisecond)))
			}
			if !event.more && slices.Equal(actual, expected) {
				return nil
			}
		}
	}
}
