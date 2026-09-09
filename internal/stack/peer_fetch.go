package stack

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// maxPeerDocument caps what is read from a machine that has not been approved.
const maxPeerDocument = 64 << 10

// FetchPeer reads what a machine says about itself.
//
// The document cannot be verified before it is read: the authority it carries
// is what an approval would be given to. What can be checked is that the
// machine answering holds that authority, so the certificate it presented is
// verified against the authority in the document. A machine that answers with
// an authority it cannot prove is refused.
func FetchPeer(address string) (PeerDocument, error) {
	var doc PeerDocument
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			// The name is an address, so there is nothing to match, and the
			// authority is not known yet. The check below replaces this one.
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	resp, err := client.Get("https://" + address + PeerPath)
	if err != nil {
		return doc, fmt.Errorf("reading %s: %w", address, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return doc, fmt.Errorf("%s answered %s", address, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPeerDocument))
	if err != nil {
		return doc, err
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return doc, fmt.Errorf("reading what %s said about itself: %w", address, err)
	}
	if doc.CA == "" {
		return doc, fmt.Errorf("%s named no authority", address)
	}
	if resp.TLS == nil || len(resp.TLS.PeerCertificates) == 0 {
		return doc, fmt.Errorf("%s presented no certificate", address)
	}
	if err := provesAuthority(resp.TLS.PeerCertificates[0], doc.CA); err != nil {
		return doc, fmt.Errorf("%s did not prove the authority it named: %w", address, err)
	}
	// The address is what this machine will connect to, not what the peer says
	// about itself.
	doc.Address = address
	return doc, nil
}

// provesAuthority reports whether a certificate was issued by the authority in
// caPEM.
func provesAuthority(cert *x509.Certificate, caPEM string) error {
	block, _ := pem.Decode([]byte(caPEM))
	if block == nil {
		return errors.New("the authority is not in PEM form")
	}
	ca, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return err
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	_, err = cert.Verify(x509.VerifyOptions{
		Roots: pool,
		// The certificate is presented for a name this machine does not know
		// yet; what is being checked is who issued it.
		KeyUsages:   []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
		CurrentTime: time.Now(),
	})
	return err
}
