package main

import (
	"context"
	"log"
	"time"

	"github.com/polyspec/bay/internal/stack"
)

// hostsCheck is how often the agent compares /etc/hosts with what the proxy
// serves and with the service containers' addresses. A command that starts a
// service waits for its name here, so this is short; a peer's domains are read
// every domainRefresh.
const hostsCheck = 2 * time.Second

// keepHostsFile keeps the names the proxy serves in /etc/hosts, on a machine
// that has no per-domain resolver. It writes only when the names change.
func keepHostsFile(ctx context.Context, m *stack.Machine) {
	t := time.NewTicker(hostsCheck)
	defer t.Stop()
	for {
		entries, err := stack.HostsEntries(m)
		if err == nil {
			if wrote, err := stack.SyncHostsFile(entries); err != nil {
				log.Printf("hosts: %v", err)
			} else if wrote {
				log.Printf("hosts: named %d name(s), %d of them service containers", len(entries), stack.ServiceHostNames(entries))
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
