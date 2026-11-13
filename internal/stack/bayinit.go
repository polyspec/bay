package stack

import (
	"bytes"
	"embed"
	"os"
	"path/filepath"
	"runtime"
)

// A service that trusts this machine's authority starts with bayinit (see
// cmd/bayinit) in front of its own entrypoint. bayinit adds the authority to
// the image's certificate store and then runs the service's command, so the
// store stays the image's own file: a package that rewrites it, such as
// ca-certificates, installs as it does without bay. Mounting a bundle over the
// store, as bay did before, made that rewrite fail.
//
// The containers run on this machine's kernel, so bayinit is built for this
// machine's architecture; an image of another architecture runs under
// emulation in the same kernel, which runs bayinit natively.

//go:embed initbin
var initFiles embed.FS

// initBinary returns the bayinit this build carries for the architecture.
// Tests replace it.
var initBinary = func() ([]byte, bool) {
	b, err := initFiles.ReadFile("initbin/bayinit-linux-" + runtime.GOARCH)
	return b, err == nil && len(b) > 0
}

const (
	// initContainerPath is where bayinit is mounted in a container.
	initContainerPath = "/etc/bay/init"
	// caContainerPath is the authority alone, which bayinit adds to the store.
	caContainerPath = "/etc/bay/ca.crt"
	// bundleContainerPath is the system's public roots and the authority, which
	// the variables name.
	bundleContainerPath = "/etc/bay/ca-bundle.crt"
)

// InitPath is where the state directory keeps bayinit.
func InitPath(dir string) string { return filepath.Join(TrustDir(dir), "bayinit") }

// initCurrent reports whether the state directory's bayinit is this build's.
// A build without one has nothing to keep current.
func initCurrent(dir string) bool {
	want, ok := initBinary()
	if !ok {
		return true
	}
	got, err := os.ReadFile(InitPath(dir))
	return err == nil && bytes.Equal(got, want)
}

// ensureInit writes this build's bayinit into the state directory. It is
// replaced, not rewritten in place: a running container keeps executing the
// file it started with.
func ensureInit(dir string) error {
	want, ok := initBinary()
	if !ok || initCurrent(dir) {
		return nil
	}
	if err := os.MkdirAll(TrustDir(dir), 0o755); err != nil {
		return err
	}
	tmp := InitPath(dir) + ".new"
	if err := os.WriteFile(tmp, want, 0o755); err != nil {
		return err
	}
	return os.Rename(tmp, InitPath(dir))
}

// initUsable reports whether a container can be started with bayinit.
func initUsable(dir string) bool {
	if dir == "" {
		return false
	}
	_, ok := initBinary()
	if !ok {
		return false
	}
	_, err := os.Stat(InitPath(dir))
	return err == nil
}
