package stack

import (
	"fmt"
	"sort"
)

// A peer's domains are read from that peer, not from what it announces. The
// announcement is a broadcast anyone on the network can send, so it carries
// only where to look; the document behind the link is what says which domains
// the machine holds. Reading it again is how a domain added or withdrawn over
// there reaches here.
//
// RefreshPeerDomains returns whether any peer's domains changed.
func RefreshPeerDomains(dir string) (bool, error) {
	return refreshPeerDomains(dir, FetchPeer)
}

func refreshPeerDomains(dir string, fetch func(string) (PeerDocument, error)) (bool, error) {
	peers, err := Peers(dir)
	if err != nil {
		return false, err
	}
	changed := false
	var failures []string
	for _, p := range peers {
		doc, err := fetch(p.Address)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", p.Name, err))
			continue
		}
		// The machine at that address has to be the machine that was approved.
		// An address that answers with another authority is another machine,
		// and what it says about its domains is not this peer's to change.
		fingerprint, err := Fingerprint(doc.CA)
		if err != nil || fingerprint != p.Fingerprint {
			failures = append(failures,
				fmt.Sprintf("%s: %s answers with another authority", p.Name, p.Address))
			continue
		}
		if sameDomains(p.Domains, doc.Domains) {
			continue
		}
		updated := p
		updated.Domains = doc.Domains
		if doc.Name != "" {
			updated.Name = doc.Name
		}
		if err := ApprovePeer(dir, updated); err != nil {
			return changed, err
		}
		changed = true
	}
	if len(failures) == len(peers) && len(peers) > 0 {
		return changed, fmt.Errorf("no approved machine answered: %v", failures)
	}
	return changed, nil
}

// sameDomains reports whether two lists hold the same names. The order a
// machine states them in is not part of what it holds.
func sameDomains(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x := append([]string(nil), a...)
	y := append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}
