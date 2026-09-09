package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/polyspec/bay/internal/stack"
)

// The resident agent is what keeps a machine findable and its peers reachable.
// It announces this machine while the link is open, and it follows an approved
// peer that comes back at another address, which is what happens when the
// network hands out a different one.
func servePeers(ctx context.Context, dir string) {
	m := stack.NewMachine(dir)
	go announceSelf(ctx, m)
	go followPeers(ctx, m)
}

// announceSelf broadcasts this machine while its link is open. It reads the
// setting before every announcement, so opening or closing the link takes
// effect without restarting the agent.
func announceSelf(ctx context.Context, m *stack.Machine) {
	announcer := stack.Announcer{OnError: func(err error) {
		log.Printf("announcing this machine: %v", err)
	}}
	err := announcer.Run(ctx, func() (stack.Beacon, bool) {
		settings, err := m.Settings()
		if err != nil || !settings.Peering {
			return stack.Beacon{}, false
		}
		address := stack.PeerLinkAddress(stack.LANAddress())
		if address == "" {
			return stack.Beacon{}, false
		}
		fingerprint, err := ownFingerprint(m)
		if err != nil {
			return stack.Beacon{}, false
		}
		return stack.Beacon{ID: fingerprint, Name: stack.MachineName(), Address: address}, true
	})
	if err != nil && ctx.Err() == nil {
		log.Printf("announcing this machine: %v", err)
	}
}

// followPeers takes the address an approved peer announces when it differs from
// the one stored, and rewrites the proxy configuration for it. A peer is its
// authority, so the announcement is matched by fingerprint and the address is
// the only thing taken from it.
func followPeers(ctx context.Context, m *stack.Machine) {
	err := stack.Listen(ctx, 0, func(b stack.Beacon) {
		peers, err := stack.Peers(m.Dir)
		if err != nil {
			return
		}
		for _, p := range peers {
			if p.Fingerprint != b.ID || p.Address == b.Address || b.Address == "" {
				continue
			}
			moved := p
			moved.Address = b.Address
			if b.Name != "" {
				moved.Name = b.Name
			}
			if err := stack.ApprovePeer(m.Dir, moved); err != nil {
				log.Printf("following %s to %s: %v", p.Name, b.Address, err)
				return
			}
			if _, err := stack.SyncProxy(m); err != nil {
				log.Printf("following %s to %s: %v", p.Name, b.Address, err)
				return
			}
			log.Printf("%s answers at %s now", moved.Name, moved.Address)
			return
		}
	})
	if err != nil && ctx.Err() == nil {
		log.Printf("listening for machines: %v", err)
	}
}

// ownFingerprint returns the fingerprint of this machine's authority.
func ownFingerprint(m *stack.Machine) (string, error) {
	ca, err := os.ReadFile(filepath.Join(m.Dir, "ca.crt"))
	if err != nil {
		return "", err
	}
	return stack.Fingerprint(string(ca))
}

// peerPollDelay keeps the agent from reading the machine state before it exists
// on a machine that has just been set up.
const peerPollDelay = 2 * time.Second
