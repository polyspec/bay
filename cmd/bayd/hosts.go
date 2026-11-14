package main

import (
	"context"
	"log"
	"time"

	"github.com/polyspec/bay/internal/stack"
)

const (
	// hostsSettle gathers the events of one change, such as a project of
	// several services coming up, into one write.
	hostsSettle = 100 * time.Millisecond
	// hostsResync reads everything again without an event, for a change no
	// event reports: a peer approved or removed by a command, or the hosts
	// file rewritten by another program.
	hostsResync = time.Minute
	// eventsRetry is the wait before the event stream is opened again after
	// it ended, as it does when the Docker daemon restarts.
	eventsRetry = 2 * time.Second
)

// hostsChanged is signalled by whatever changes the names: a Docker event,
// and new domains read from an approved peer.
var hostsChanged = make(chan struct{}, 1)

func nudgeHosts() {
	select {
	case hostsChanged <- struct{}{}:
	default:
	}
}

// keepHostsFile keeps the names the proxy serves and the service containers'
// names in /etc/hosts, on a machine that has no per-domain resolver. It
// rewrites the block when Docker reports that a container started, ended, was
// removed or changed networks, and when an approved peer's domains change.
// It writes only when the entries change.
func keepHostsFile(ctx context.Context, m *stack.Machine) {
	go func() {
		for ctx.Err() == nil {
			if err := stack.WatchDockerEvents(ctx, nudgeHosts, nudgeHosts); err != nil && ctx.Err() == nil {
				log.Printf("hosts: %v; reading again in %s", err, eventsRetry)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(eventsRetry):
			}
		}
	}()
	resync := time.NewTicker(hostsResync)
	defer resync.Stop()
	for {
		syncHosts(m)
		select {
		case <-ctx.Done():
			return
		case <-hostsChanged:
			time.Sleep(hostsSettle)
		case <-resync.C:
		}
	}
}

func syncHosts(m *stack.Machine) {
	entries, err := stack.HostsEntries(m)
	if err != nil {
		log.Printf("hosts: %v", err)
		return
	}
	if wrote, err := stack.SyncHostsFile(entries); err != nil {
		log.Printf("hosts: %v", err)
	} else if wrote {
		log.Printf("hosts: named %d name(s), %d of them service containers", len(entries), stack.ServiceHostNames(entries))
	}
}
