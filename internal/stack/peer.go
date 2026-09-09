package stack

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Peer is another machine running bay, reachable on this network.
//
// A peer is what its authority is: the fingerprint is the identity. Name,
// Address and Domains are read from the peer and change without making it a
// different peer, so nothing is decided by them.
type Peer struct {
	// Fingerprint identifies the authority. It is filled in from CA.
	Fingerprint string `json:"fingerprint"`
	Name        string `json:"name"`
	// Address is where the peer's link answers, as host:port.
	Address string   `json:"address"`
	Domains []string `json:"domains,omitempty"`
	// CA is the peer's certificate authority, in PEM. It is what a certificate
	// the peer presents is checked against.
	CA string `json:"ca"`
}

// peersFile holds the approved peers.
func peersFile(dir string) string { return filepath.Join(dir, "peers.json") }

// Fingerprint returns the identity of an authority given its certificate in
// PEM: the SHA-256 of the certificate as the issuer signed it.
func Fingerprint(caPEM string) (string, error) {
	block, _ := pem.Decode([]byte(caPEM))
	if block == nil || block.Type != "CERTIFICATE" {
		return "", errors.New("not a certificate in PEM form")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("reading the authority: %w", err)
	}
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:]), nil
}

// Peers returns the approved peers, ordered by name and then by fingerprint so
// two peers sharing a name keep a stable order.
func Peers(dir string) ([]Peer, error) {
	b, err := os.ReadFile(peersFile(dir))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var byFingerprint map[string]Peer
	if err := json.Unmarshal(b, &byFingerprint); err != nil {
		return nil, fmt.Errorf("reading %s: %w", peersFile(dir), err)
	}
	out := make([]Peer, 0, len(byFingerprint))
	for fingerprint, p := range byFingerprint {
		p.Fingerprint = fingerprint
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Fingerprint < out[j].Fingerprint
	})
	return out, nil
}

// ApprovePeer stores a peer under its authority's fingerprint. Approving one
// that is already stored takes its new name, address and domains: the machine
// is the same one.
func ApprovePeer(dir string, p Peer) error {
	fingerprint, err := Fingerprint(p.CA)
	if err != nil {
		return err
	}
	p.Fingerprint = fingerprint
	return writePeers(dir, func(byFingerprint map[string]Peer) {
		byFingerprint[fingerprint] = p
	})
}

// RemovePeer withdraws the approval given to an authority.
func RemovePeer(dir, fingerprint string) error {
	return writePeers(dir, func(byFingerprint map[string]Peer) {
		delete(byFingerprint, fingerprint)
	})
}

// writePeers reads the file, applies the change and writes it back.
func writePeers(dir string, change func(map[string]Peer)) error {
	byFingerprint := map[string]Peer{}
	b, err := os.ReadFile(peersFile(dir))
	switch {
	case err == nil:
		if err := json.Unmarshal(b, &byFingerprint); err != nil {
			return fmt.Errorf("reading %s: %w", peersFile(dir), err)
		}
	case !os.IsNotExist(err):
		return err
	}
	change(byFingerprint)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	out, err := json.MarshalIndent(byFingerprint, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(peersFile(dir), append(out, '\n'), 0o644)
}

// PeerDomains returns every domain the approved peers serve, without the ones
// this machine serves itself: a name this machine serves is always this
// machine's.
func PeerDomains(peers []Peer, own []string) map[string]Peer {
	mine := make(map[string]bool, len(own))
	for _, d := range own {
		mine[strings.ToLower(d)] = true
	}
	out := map[string]Peer{}
	for _, p := range peers {
		for _, d := range p.Domains {
			d = strings.ToLower(d)
			if mine[d] {
				continue
			}
			// Two peers offering one name: the first in order keeps it, so the
			// answer does not depend on which peer was read first.
			if _, taken := out[d]; taken {
				continue
			}
			out[d] = p
		}
	}
	return out
}

// readFile returns a file's contents as a string.
func readFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	return string(b), err
}
