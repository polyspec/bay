// Command bayinit runs first in a service container that trusts this
// machine's authority. It adds the authority, mounted at /etc/bay/ca.crt, to
// the image's own certificate store and to the directories the image's tools
// rebuild that store from, then replaces itself with the service's command.
//
// The store stays the image's file, so installing or updating the
// ca-certificates package rewrites it as usual and keeps the authority. A
// container whose user cannot write the store, or whose root file system is
// read-only, starts unchanged and trusts the authority through the variables
// bay sets (SSL_CERT_FILE and the others).
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const authority = "/etc/bay/ca.crt"

// stores are the bundles images read: Debian, Ubuntu and Alpine; Fedora and
// RHEL; Alpine's LibreSSL link; the RHEL extracted bundle.
var stores = []string{
	"/etc/ssl/certs/ca-certificates.crt",
	"/etc/pki/tls/certs/ca-bundle.crt",
	"/etc/ssl/cert.pem",
	"/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem",
}

// sources are the directories update-ca-certificates and update-ca-trust
// build the stores from.
var sources = []string{
	"/usr/local/share/ca-certificates/bay.crt",
	"/etc/pki/ca-trust/source/anchors/bay.crt",
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "bayinit: no command to run")
		os.Exit(127)
	}
	if ca, err := os.ReadFile(authority); err == nil {
		trust(ca)
	}
	path := os.Args[1]
	if !strings.Contains(path, "/") {
		found, err := exec.LookPath(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bayinit: %v\n", err)
			os.Exit(127)
		}
		path = found
	}
	err := syscall.Exec(path, os.Args[1:], os.Environ())
	fmt.Fprintf(os.Stderr, "bayinit: %s: %v\n", os.Args[1], err)
	os.Exit(126)
}

// trust adds the authority where it is missing. Failures are not reported:
// the variables still name a bundle that holds it.
func trust(ca []byte) {
	want := flat(ca)
	seen := map[string]bool{}
	for _, p := range stores {
		real, err := filepath.EvalSymlinks(p)
		if err != nil || seen[real] {
			continue
		}
		seen[real] = true
		b, err := os.ReadFile(real)
		if err != nil || bytes.Contains(flat(b), want) {
			continue
		}
		f, err := os.OpenFile(real, os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			continue
		}
		if len(b) > 0 && b[len(b)-1] != '\n' {
			f.Write([]byte("\n"))
		}
		f.Write(ca)
		f.Close()
	}
	for _, p := range sources {
		if info, err := os.Stat(filepath.Dir(p)); err != nil || !info.IsDir() {
			continue
		}
		if b, err := os.ReadFile(p); err == nil && bytes.Equal(b, ca) {
			continue
		}
		os.WriteFile(p, ca, 0o644)
	}
}

// flat removes whitespace, so a certificate compares equal however its lines
// are broken.
func flat(b []byte) []byte {
	return []byte(strings.Join(strings.Fields(string(b)), ""))
}
