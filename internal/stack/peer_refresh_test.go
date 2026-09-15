package stack

import (
	"errors"
	"os"
	"testing"
)

// approvedPeer stores one peer and returns its authority in PEM.
func approvedPeer(t *testing.T, dir string, domains ...string) (Peer, string) {
	t.Helper()
	ca, err := LoadOrCreateCA(dir)
	if err != nil {
		t.Fatal(err)
	}
	pem, err := os.ReadFile(ca.CertPath())
	if err != nil {
		t.Fatal(err)
	}
	p := Peer{Name: "max", Address: "192.168.0.59:8443", Domains: domains, CA: string(pem)}
	if err := ApprovePeer(dir, p); err != nil {
		t.Fatal(err)
	}
	stored, err := Peers(dir)
	if err != nil || len(stored) != 1 {
		t.Fatalf("the peer was not stored: %v %v", stored, err)
	}
	return stored[0], string(pem)
}

// A domain added on the other machine reaches here. The announcement carries no
// domains, so without reading the document again the list stays as it was when
// the machine was approved and the new name is never served.
func TestADomainAddedOnThePeerReachesHere(t *testing.T) {
	dir := t.TempDir()
	p, pem := approvedPeer(t, dir, "polyspec.test")

	changed, err := refreshPeerDomains(dir, func(address string) (PeerDocument, error) {
		if address != p.Address {
			t.Fatalf("the document was read from %s, want %s", address, p.Address)
		}
		return PeerDocument{Name: "max", Address: p.Address, CA: pem,
			Domains: []string{"polyspec.test", "registry.soksak.test"}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("a domain was added on the peer and nothing changed here")
	}
	after, err := Peers(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(after[0].Domains) != 2 {
		t.Fatalf("the domains are %v, want both", after[0].Domains)
	}
}

// A domain withdrawn over there stops being held here, so it stops being served
// and its certificate stops being counted as in use.
func TestADomainWithdrawnOnThePeerIsDroppedHere(t *testing.T) {
	dir := t.TempDir()
	p, pem := approvedPeer(t, dir, "polyspec.test", "registry.soksak.test")

	if _, err := refreshPeerDomains(dir, func(string) (PeerDocument, error) {
		return PeerDocument{Name: "max", Address: p.Address, CA: pem,
			Domains: []string{"polyspec.test"}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	after, _ := Peers(dir)
	if len(after[0].Domains) != 1 || after[0].Domains[0] != "polyspec.test" {
		t.Fatalf("the domains are %v, want only the one still held", after[0].Domains)
	}
}

// The same domains in another order are the same domains, so nothing is
// rewritten and the proxy is not asked to reload for it.
func TestTheSameDomainsInAnotherOrderChangeNothing(t *testing.T) {
	dir := t.TempDir()
	p, pem := approvedPeer(t, dir, "a.test", "b.test")

	changed, err := refreshPeerDomains(dir, func(string) (PeerDocument, error) {
		return PeerDocument{Name: "max", Address: p.Address, CA: pem,
			Domains: []string{"b.test", "a.test"}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("the same domains in another order were taken as a change")
	}
}

// An address that answers with another authority is another machine. What it
// says about its domains is not this peer's to change.
func TestAnotherAuthorityAtTheAddressChangesNothing(t *testing.T) {
	dir := t.TempDir()
	approvedPeer(t, dir, "polyspec.test")
	other, err := LoadOrCreateCA(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	otherPEM, err := os.ReadFile(other.CertPath())
	if err != nil {
		t.Fatal(err)
	}

	changed, err := refreshPeerDomains(dir, func(string) (PeerDocument, error) {
		return PeerDocument{Name: "max", CA: string(otherPEM),
			Domains: []string{"taken.test"}}, nil
	})
	if err == nil {
		t.Fatal("an address answering with another authority was accepted")
	}
	if changed {
		t.Error("another machine changed this peer's domains")
	}
	after, _ := Peers(dir)
	if len(after[0].Domains) != 1 || after[0].Domains[0] != "polyspec.test" {
		t.Fatalf("the domains are %v, want what was approved", after[0].Domains)
	}
}

// A peer that cannot be reached leaves what is held alone: it has not given up
// a domain, it has not been asked.
func TestAPeerThatDoesNotAnswerKeepsItsDomains(t *testing.T) {
	dir := t.TempDir()
	approvedPeer(t, dir, "polyspec.test")

	changed, err := refreshPeerDomains(dir, func(string) (PeerDocument, error) {
		return PeerDocument{}, errors.New("no route to host")
	})
	if err == nil {
		t.Fatal("a peer that did not answer was not reported")
	}
	if changed {
		t.Error("a peer that did not answer changed what is held")
	}
	after, _ := Peers(dir)
	if len(after[0].Domains) != 1 {
		t.Fatalf("the domains are %v, want what was approved", after[0].Domains)
	}
}
