package main

import (
	"context"
	"log"
	"time"

	"github.com/polyspec/bay/internal/stack"
)

// hostsCheck is how often the agent compares /etc/hosts with what the proxy
// serves. A peer's domains are read every domainRefresh, and a project's when
// it comes up, so this follows both within seconds.
const hostsCheck = 10 * time.Second

// keepHostsFile keeps the names the proxy serves in /etc/hosts, on a machine
// that has no per-domain resolver. It writes only when the names change.
func keepHostsFile(ctx context.Context, m *stack.Machine) {
	t := time.NewTicker(hostsCheck)
	defer t.Stop()
	for {
		names, err := stack.HostsNames(m)
		if err == nil {
			if wrote, err := stack.SyncHostsFile(names); err != nil {
				log.Printf("hosts: %v", err)
			} else if wrote {
				log.Printf("hosts: named %d domain(s)", len(names))
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
