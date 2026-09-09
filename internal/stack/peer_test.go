package stack

import (
	"path/filepath"
	"testing"
)

// A peer is stored under the fingerprint of its authority. Its name and address
// are attributes: a peer that changed both is the same peer.
func TestAPeerIsTheSameWhenOnlyItsNameAndAddressChange(t *testing.T) {
	dir := t.TempDir()
	ca := testCA(t)

	first := Peer{Name: "alpha", Address: "192.168.0.10:8443", Domains: []string{"web.test"}, CA: ca}
	if err := ApprovePeer(dir, first); err != nil {
		t.Fatal(err)
	}
	moved := Peer{Name: "renamed", Address: "192.168.0.99:8443", Domains: []string{"web.test", "api.test"}, CA: ca}
	if err := ApprovePeer(dir, moved); err != nil {
		t.Fatal(err)
	}

	peers, err := Peers(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 1 {
		t.Fatalf("%d peers after the name and the address changed, want 1", len(peers))
	}
	if peers[0].Name != "renamed" || peers[0].Address != "192.168.0.99:8443" {
		t.Errorf("the attributes were not taken: %+v", peers[0])
	}
	if len(peers[0].Domains) != 2 {
		t.Errorf("the domains were not taken: %v", peers[0].Domains)
	}
}

// Another authority is another peer, whatever it calls itself.
func TestAnotherAuthorityIsAnotherPeer(t *testing.T) {
	dir := t.TempDir()
	for _, ca := range []string{testCA(t), testCA(t)} {
		if err := ApprovePeer(dir, Peer{Name: "alpha", Address: "192.168.0.10:8443", CA: ca}); err != nil {
			t.Fatal(err)
		}
	}
	peers, err := Peers(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 2 {
		t.Fatalf("%d peers for two authorities under one name, want 2", len(peers))
	}
	if peers[0].Fingerprint == peers[1].Fingerprint {
		t.Error("two authorities produced one fingerprint")
	}
}

// Removing takes the fingerprint, because that is what a peer is.
func TestAPeerIsRemovedByItsFingerprint(t *testing.T) {
	dir := t.TempDir()
	ca := testCA(t)
	if err := ApprovePeer(dir, Peer{Name: "alpha", CA: ca}); err != nil {
		t.Fatal(err)
	}
	peers, _ := Peers(dir)
	if err := RemovePeer(dir, peers[0].Fingerprint); err != nil {
		t.Fatal(err)
	}
	if peers, _ = Peers(dir); len(peers) != 0 {
		t.Errorf("%d peers after removal, want 0", len(peers))
	}
}

// A machine with no peers reads as none rather than as an error.
func TestNoPeersIsNotAnError(t *testing.T) {
	peers, err := Peers(filepath.Join(t.TempDir(), "empty"))
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 0 {
		t.Errorf("%d peers on a machine that has none", len(peers))
	}
}

// A fingerprint identifies the authority, so it has to be the same every time
// the same certificate is read.
func TestTheFingerprintIsStable(t *testing.T) {
	ca := testCA(t)
	a, err := Fingerprint(ca)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Fingerprint(ca)
	if err != nil {
		t.Fatal(err)
	}
	if a != b || a == "" {
		t.Errorf("fingerprints %q and %q", a, b)
	}
}

// testCA returns the certificate of a new authority, in PEM.
func testCA(t *testing.T) string {
	t.Helper()
	ca, err := LoadOrCreateCA(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pem, err := readFile(ca.CertPath())
	if err != nil {
		t.Fatal(err)
	}
	return pem
}

// A name this machine serves is always this machine's, whatever a peer offers.
func TestOwnDomainsWinOverAPeers(t *testing.T) {
	peers := []Peer{{Fingerprint: "aa", Name: "alpha", Domains: []string{"web.test", "api.test"}}}
	got := PeerDomains(peers, []string{"web.test"})
	if _, taken := got["web.test"]; taken {
		t.Error("a name this machine serves was answered from a peer")
	}
	if _, ok := got["api.test"]; !ok {
		t.Error("a name only the peer serves was not answered from it")
	}
}

// Case is not part of a domain.
func TestOwnDomainsWinWithoutRegardToCase(t *testing.T) {
	peers := []Peer{{Fingerprint: "aa", Domains: []string{"WEB.TEST"}}}
	if _, taken := PeerDomains(peers, []string{"web.test"})["web.test"]; taken {
		t.Error("a name this machine serves was answered from a peer in another case")
	}
}

// Two peers offering one name: the first in order keeps it, so the answer does
// not depend on which peer was read first.
func TestTwoPeersOfferingOneNameAreOrdered(t *testing.T) {
	peers := []Peer{
		{Fingerprint: "aa", Name: "alpha", Domains: []string{"shared.test"}},
		{Fingerprint: "bb", Name: "beta", Domains: []string{"shared.test"}},
	}
	if got := PeerDomains(peers, nil)["shared.test"]; got.Name != "alpha" {
		t.Errorf("shared.test answered from %q, want the first in order", got.Name)
	}
}

// An address that already carries an approved authority, now carrying another
// one, is not the same machine. Approving it silently would put two entries
// under one address and give the new one everything the old one had.
func TestAnApprovedAddressCarryingAnotherAuthorityIsReported(t *testing.T) {
	dir := t.TempDir()
	first := Peer{Name: "alpha", Address: "192.168.0.10:8443", CA: testCA(t)}
	if err := ApprovePeer(dir, first); err != nil {
		t.Fatal(err)
	}
	peers, _ := Peers(dir)

	other := Peer{Name: "alpha", Address: "192.168.0.10:8443", CA: testCA(t)}
	known, err := PeerAtAddress(peers, other.Address, other.CA)
	if err != nil {
		t.Fatal(err)
	}
	if known == nil {
		t.Fatal("the address is not reported as one that already carries an authority")
	}
	if known.Fingerprint != peers[0].Fingerprint {
		t.Errorf("reported %s, want the authority already approved", known.Fingerprint)
	}
}

// The same machine answering again at the same address is not a change.
func TestTheSameAuthorityAtAKnownAddressIsNotReported(t *testing.T) {
	dir := t.TempDir()
	ca := testCA(t)
	if err := ApprovePeer(dir, Peer{Name: "alpha", Address: "192.168.0.10:8443", CA: ca}); err != nil {
		t.Fatal(err)
	}
	peers, _ := Peers(dir)
	known, err := PeerAtAddress(peers, "192.168.0.10:8443", ca)
	if err != nil {
		t.Fatal(err)
	}
	if known != nil {
		t.Error("a machine answering with the authority it was approved under is reported as changed")
	}
}
