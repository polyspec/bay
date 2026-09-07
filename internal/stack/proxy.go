package stack

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// SyncResult reports what SyncProxy changed.
type SyncResult struct {
	Routes    []Route
	Conflicts []DomainConflict
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

	if len(routes) == 0 {
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
	generation := routeGeneration(routes)
	if err := RenderNginx(m.ConfDir(), routes, gateway, DefaultCertName, generation); err != nil {
		return res, err
	}

	created, err := EnsureProxy(m.ConfDir(), m.CertDir())
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

// routeGeneration returns a stable value for the route list. The health
// endpoint reports it, so a caller can tell which configuration is running.
func routeGeneration(routes []Route) string {
	h := sha256.New()
	for _, r := range routes {
		fmt.Fprintf(h, "%s\x00%s\x00%s\n", r.Domain, r.Scheme, r.Backend)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// WaitForGeneration polls the proxy's health endpoint until it reports the
// configuration identified by generation, or the timeout expires.
func WaitForGeneration(generation string, timeout time.Duration) error {
	client := &http.Client{Timeout: 2 * time.Second}
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
