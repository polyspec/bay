//go:build !darwin || !cgo

package stack

import (
	"context"
	"errors"
	"net/netip"
)

func nativeNameSubscription(context.Context, string, []netip.Addr) (*nameSubscription, error) {
	return nil, errors.New("Apple hostname subscriptions require macOS with cgo")
}
