package stack

import (
	"crypto/sha256"
	"crypto/tls"
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
	// The proxy is one container per engine and it serves this state
	// directory's configuration and certificates, so a state directory that
	// does not own the machine setup does not replace it.
	if err := OwnsMachineSetup(m.Dir); err != nil {
		return SyncResult{}, err
	}
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

	settings, err := m.Settings()
	if err != nil {
		return res, err
	}
	engines := Engines()
	if len(engines) == 0 {
		return res, fmt.Errorf("no container engine found: install Apple %s or %s",
			AppleEngine, DockerEngine)
	}

	// The link is open before there is any peer: a machine has to answer the
	// document another machine approves it from.
	linkEngine := ""
	if settings.Peering || len(peers) > 0 {
		if err := peerLinkServable(); err != nil {
			return res, err
		}
		linkEngine = engines[0]
		if err := preparePeerLink(m, peers, routes, linkEngine); err != nil {
			return res, err
		}
	}

	stopped := 0
	for _, engine := range engines {
		// A peer's domain has no container, so every proxy serves it. The proxy
		// that publishes the link runs with no route, because it returns the
		// document used to approve this machine.
		own := RoutesOn(routes, engine)
		if len(own) == 0 && len(peerRoutes) == 0 && engine != linkEngine {
			if err := StopProxy(engine); err != nil {
				return res, err
			}
			stopped++
			continue
		}
		if err := issueCertificates(m, own); err != nil {
			return res, err
		}
		conf := NginxConfig{
			Routes:      own,
			DefaultCert: DefaultCertName,
			PeerRoutes:  peerRoutes,
			LinkLog:     settings.LinkLog,
		}
		// The resolver is used only by a route's name fallback, so a
		// configuration with no route needs none.
		if len(own) > 0 {
			gateway, err := NetworkGateway(engine)
			if err != nil {
				return res, err
			}
			conf.Resolver = engineResolver(engine, gateway)
		}
		peerDir := ""
		if engine == linkEngine {
			peerDir = PeerDir(m.Dir)
			conf.PeerPort = PeerPort
			if len(peers) > 0 {
				conf.PeerAuthorities = "/etc/nginx/peers/" + PeerAuthoritiesName
			}
			conf.ClientCert = "/etc/nginx/peers/" + ClientCertName + ".crt"
			conf.ClientKey = "/etc/nginx/peers/" + ClientCertName + ".key"
		}
		conf.Generation = configGeneration(conf)
		if err := RenderNginxConfig(m.ConfDir(engine), conf); err != nil {
			return res, err
		}
		created, err := EnsureProxy(engine, m.ConfDir(engine), m.CertDir(), peerDir)
		if err != nil {
			return res, err
		}
		res.Action = "reloaded"
		if created {
			res.Action = "started"
		} else if err := ReloadProxy(engine); err != nil {
			return res, err
		}
		// Return only after the proxy serves the new configuration.
		if err := WaitForGeneration(engine, conf.Generation, 60*time.Second); err != nil {
			return res, err
		}
		if err := waitReachable(engine, 30*time.Second); err != nil {
			return res, err
		}
	}
	if stopped == len(engines) {
		res.Action = "stopped"
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

// peerLinkServable reports whether one engine's proxy can serve all of
// the peer link. The link is one port and a proxy serves only its own engine's
// containers, so a machine running both engines would answer some of its own
// names on the link and none of the rest. Rule 8 of the architecture puts the
// link on a host process that reaches both engines; until that exists the link
// is refused rather than served incompletely.
func peerLinkServable() error {
	engines := Engines()
	if len(engines) > 1 {
		return fmt.Errorf(
			"the peer link is one port and this machine runs %d engines; "+
				"it belongs to a host process that reaches both, which is not built",
			len(engines))
	}
	if len(engines) == 0 {
		return fmt.Errorf("no container engine found: install Apple %s or %s",
			AppleEngine, DockerEngine)
	}
	return nil
}

// preparePeerLink writes what the proxy reads to run the link: this machine's
// client certificate, the approved authorities, the certificates for the
// domains peers serve, and the document a machine is approved from.
func preparePeerLink(m *Machine, peers []Peer, routes []Route, engine string) error {
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
	return WritePeerDocument(m.ConfDir(engine), PeerDocument{
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

// waitReachable confirms the port the proxy serves names on answers at the
// address the host reaches it by. The health endpoint answers on another port,
// so it says nothing about this one: a machine has been left with the proxy
// running and answering there while the port serving every name was reset on
// connection. When the container answers and the published port does not, the
// container is started again, which is what rebuilds the engine's forwarding.
func waitReachable(engine string, timeout time.Duration) error {
	err := proxyReachable(engine)
	if err == nil {
		return nil
	}
	if _, restartErr := runEngine(engine, "restart", ProxyName); restartErr != nil {
		return fmt.Errorf("%s serves no name at its published port and did not restart: %w", ProxyName, err)
	}
	deadline := time.Now().Add(timeout)
	for {
		if err = proxyReachable(engine); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s serves no name at its published port after a restart: %w", ProxyName, err)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// proxyReachable opens the port the proxy serves names on, from where the host
// reaches it, and completes the handshake. The certificate is not checked: what
// is being asked is whether the port answers at all.
func proxyReachable(engine string) error {
	in, found, err := lookupOn(engine, ProxyName)
	if err != nil {
		return err
	}
	addr := ProxyHostAddr(engine, in)
	if !found || in.State != "running" || addr == "" {
		return fmt.Errorf("%s is not running", ProxyName)
	}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 3 * time.Second}, "tcp",
		net.JoinHostPort(addr, "443"), &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		return err
	}
	return conn.Close()
}

// WaitForGeneration polls the proxy's health endpoint until it reports the
// configuration identified by generation, or the timeout expires.
func WaitForGeneration(engine, generation string, timeout time.Duration) error {
	return waitForGeneration(engine, generation, timeout, &http.Client{Timeout: 2 * time.Second})
}

func waitForGeneration(engine, generation string, timeout time.Duration, client *http.Client) error {
	deadline := time.Now().Add(timeout)
	var last string
	for {
		in, found, err := lookupOn(engine, ProxyName)
		if err != nil {
			return err
		}
		if addr := ProxyHostAddr(engine, in); found && in.State == "running" && addr != "" {
			got, err := fetchGeneration(client, addr)
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
