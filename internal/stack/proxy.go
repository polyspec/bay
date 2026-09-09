package stack

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

// SyncResult reports what SyncProxy changed.
type SyncResult struct {
	Routes    []Route
	Conflicts []DomainConflict
	// Peers are the approved peers the configuration was written with.
	Peers []Peer
	// Action is "started", "reloaded" or "stopped".
	Action string
}

// SyncProxy rebuilds the proxy configuration from the labels on every running
// service container, issues any certificate it is missing, and applies it.
//
// The configuration is generated from the running containers, so several
// projects share one proxy and removing a project removes its routes. The proxy
// is removed when no route remains.
func SyncProxy(m *Machine) (SyncResult, error) {
	routes, conflicts, err := Routes()
	if err != nil {
		return SyncResult{}, err
	}
	res := SyncResult{Routes: routes, Conflicts: conflicts}

	peers, err := Peers(m.Dir)
	if err != nil {
		return res, err
	}
	own := make([]string, 0, len(routes))
	for _, r := range routes {
		own = append(own, r.Domain)
	}
	peerRoutes := peerRoutesFor(m, peers, own)
	res.Peers = peers

	if len(routes) == 0 && len(peerRoutes) == 0 {
		if err := StopProxy(); err != nil {
			return res, err
		}
		res.Action = "stopped"
		return res, nil
	}

	if err := issueCertificates(m, routes); err != nil {
		return res, err
	}
	gateway, err := NetworkGateway()
	if err != nil {
		return res, err
	}
	conf := NginxConfig{
		Routes:      routes,
		Resolver:    gateway,
		DefaultCert: DefaultCertName,
		PeerRoutes:  peerRoutes,
	}
	settings, err := m.Settings()
	if err != nil {
		return res, err
	}
	peerDir := ""
	// The link is open before there is any peer: a machine has to answer the
	// document another machine approves it from.
	if settings.Peering || len(peers) > 0 {
		if err := preparePeerLink(m, peers, routes); err != nil {
			return res, err
		}
		peerDir = PeerDir(m.Dir)
		conf.PeerPort = PeerPort
		if len(peers) > 0 {
			conf.PeerAuthorities = "/etc/nginx/peers/" + PeerAuthoritiesName
		}
		conf.ClientCert = "/etc/nginx/peers/" + ClientCertName + ".crt"
		conf.ClientKey = "/etc/nginx/peers/" + ClientCertName + ".key"
	}
	conf.Generation = configGeneration(conf)
	generation := conf.Generation
	if err := RenderNginxConfig(m.ConfDir(), conf); err != nil {
		return res, err
	}

	created, err := EnsureProxy(m.ConfDir(), m.CertDir(), peerDir)
	if err != nil {
		return res, err
	}
	res.Action = "reloaded"
	if created {
		res.Action = "started"
	} else if err := ReloadProxy(); err != nil {
		return res, err
	}
	// Return only after the proxy serves the new configuration.
	if err := WaitForGeneration(generation, 60*time.Second); err != nil {
		return res, err
	}
	return res, nil
}

// peerRoutesFor returns the domains approved peers serve that this machine does
// not serve itself. Every one of them is answered here, with a certificate this
// machine issues, and forwarded over that peer's link.
func peerRoutesFor(m *Machine, peers []Peer, own []string) []PeerRoute {
	byDomain := PeerDomains(peers, own)
	domains := make([]string, 0, len(byDomain))
	for d := range byDomain {
		domains = append(domains, d)
	}
	sort.Strings(domains)
	out := make([]PeerRoute, 0, len(domains))
	for _, d := range domains {
		p := byDomain[d]
		out = append(out, PeerRoute{
			Domain:    d,
			Address:   p.Address,
			Authority: "/etc/nginx/peers/" + p.Fingerprint + ".crt",
		})
	}
	return out
}

// preparePeerLink writes what the proxy reads to run the link: this machine's
// client certificate, the approved authorities, the certificates for the
// domains peers serve, and the document a machine is approved from.
func preparePeerLink(m *Machine, peers []Peer, routes []Route) error {
	ca, err := LoadOrCreateCA(m.Dir)
	if err != nil {
		return err
	}
	if err := ca.IssueClient(PeerDir(m.Dir), MachineName()); err != nil {
		return err
	}
	if err := WritePeerAuthorities(m.Dir, peers); err != nil {
		return err
	}
	own := make([]string, 0, len(routes))
	for _, r := range routes {
		own = append(own, r.Domain)
	}
	// A peer's domain is served here with a certificate this machine issues, so
	// a browser is offered one from the authority this machine already trusts.
	for d := range PeerDomains(peers, own) {
		if _, err := ca.Issue(m.CertDir(), d); err != nil {
			return err
		}
	}
	caPEM, err := os.ReadFile(ca.CertPath())
	if err != nil {
		return err
	}
	return WritePeerDocument(m.ConfDir(), PeerDocument{
		Name:    MachineName(),
		Address: PeerLinkAddress(LANAddress()),
		Domains: own,
		CA:      string(caPEM),
	})
}

// configGeneration identifies a configuration. It covers the peers as well as
// the routes, so approving one is a change the proxy is waited for.
func configGeneration(c NginxConfig) string {
	h := sha256.New()
	for _, r := range c.Routes {
		fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00%s\n", r.Domain, r.Scheme, r.Backend, r.ipv4, r.started)
	}
	for _, r := range c.PeerRoutes {
		fmt.Fprintf(h, "peer\x00%s\x00%s\x00%s\n", r.Domain, r.Address, r.Authority)
	}
	fmt.Fprintf(h, "link\x00%d\n", c.PeerPort)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// routeGeneration identifies routes and their current backend instances. A
// worker from before a backend restart cannot satisfy the new generation wait.
func routeGeneration(routes []Route) string {
	h := sha256.New()
	for _, r := range routes {
		fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00%s\n", r.Domain, r.Scheme, r.Backend, r.ipv4, r.started)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// WaitForGeneration polls the proxy's health endpoint until it reports the
// configuration identified by generation, or the timeout expires.
func WaitForGeneration(generation string, timeout time.Duration) error {
	return waitForGeneration(generation, timeout, &http.Client{Timeout: 2 * time.Second})
}

func waitForGeneration(generation string, timeout time.Duration, client *http.Client) error {
	deadline := time.Now().Add(timeout)
	var last string
	for {
		in, found, err := Lookup(ProxyName)
		if err != nil {
			return err
		}
		if found && in.State == "running" && in.IPv4 != "" {
			got, err := fetchGeneration(client, in.IPv4)
			if err == nil && got == generation {
				return nil
			}
			if err != nil {
				last = err.Error()
			} else {
				last = "serving generation " + got
			}
		} else {
			last = "proxy not running yet"
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("proxy did not start serving configuration %s within %s (%s)",
				generation, timeout, last)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func fetchGeneration(client *http.Client, ip string) (string, error) {
	resp, err := client.Get("http://" + net.JoinHostPort(ip, "80") + HealthPath)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 128))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// DefaultCertName is the certificate the default server presents. A name under
// a delegated domain that has no route completes the handshake and receives
// 404.
const DefaultCertName = "_default"

func issueCertificates(m *Machine, routes []Route) error {
	ca, err := LoadOrCreateCA(m.Dir)
	if err != nil {
		return err
	}
	for _, r := range routes {
		if _, err := ca.Issue(m.CertDir(), r.Domain); err != nil {
			return fmt.Errorf("issuing certificate for %s: %w", r.Domain, err)
		}
	}

	// The default certificate covers every routed domain and a wildcard per
	// delegated domain. Clients reject a wildcard whose parent is a single
	// label, so "*.test" does not apply to a name under "test".
	domains, err := m.Domains()
	if err != nil {
		return err
	}
	names := make([]string, 0, len(domains)*2+len(routes))
	for _, d := range domains {
		names = append(names, d, "*."+d)
	}
	for _, r := range routes {
		names = append(names, r.Domain)
	}
	if _, err := ca.IssueWithNames(m.CertDir(), DefaultCertName, names); err != nil {
		return fmt.Errorf("issuing the default certificate: %w", err)
	}
	return nil
}
