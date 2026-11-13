package main

import (
	"context"
	"log"
	"net"
	"time"

	"github.com/polyspec/bay/internal/stack"
)

// relayCheck is how often the agent reads the gateway of the Apple proxy's
// network, and how often the relay daemon reads where to listen. The network
// is created by the engine and can be created again with another gateway, and
// the relay has to listen on the one the proxy is written with.
const relayCheck = 30 * time.Second

// recordRelayAddress keeps the record of where the relay listens current. The
// relay runs as a launchd daemon (see stack.RelayDaemonLabel), which cannot
// read the user's Apple `container` network, so the agent reads it.
func recordRelayAddress(ctx context.Context, m *stack.Machine) {
	t := time.NewTicker(relayCheck)
	defer t.Stop()
	for {
		if address := stack.PeerRelayAddress(); address != "" {
			if err := stack.RecordRelayAddress(m.Dir, address); err != nil {
				log.Printf("relay: recording %s: %v", address, err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// serveRelay is the relay daemon: "bayd -relay -state dir". It listens
// where the agent recorded, moves when the record changes, and relays a name to
// the approved peer of that state directory that serves it.
func serveRelay(ctx context.Context, dir string) {
	m := stack.NewMachine(dir)
	t := time.NewTicker(relayCheck / 6)
	defer t.Stop()
	var current *relayListener
	defer func() { current.close() }()
	for {
		if want := stack.RecordedRelayAddress(dir); want != current.address() {
			current.close()
			current = openRelay(ctx, m, want)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// relayListener is the relay listening on one address.
type relayListener struct {
	addr   string
	cancel context.CancelFunc
}

// openRelay listens on address and serves the relay there. It returns nothing
// when address is empty or cannot be listened on; the next check tries again.
func openRelay(ctx context.Context, m *stack.Machine, address string) *relayListener {
	if address == "" {
		return nil
	}
	ln, err := net.Listen("tcp", address)
	if err != nil {
		log.Printf("relay: listening on %s: %v", address, err)
		return nil
	}
	relayCtx, cancel := context.WithCancel(ctx)
	log.Printf("relay: listening on %s", address)
	go stack.ServeRelay(relayCtx, ln,
		func(name string) string { return stack.RelayTarget(m, name) },
		log.Printf)
	return &relayListener{addr: address, cancel: cancel}
}

func (r *relayListener) address() string {
	if r == nil {
		return ""
	}
	return r.addr
}

func (r *relayListener) close() {
	if r == nil {
		return
	}
	r.cancel()
	log.Printf("relay: stopped listening on %s", r.addr)
}
