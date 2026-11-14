package stack

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeContext writes a docker context the way the docker command stores one.
func writeContext(t *testing.T, dir, name, host string) {
	t.Helper()
	sum := sha256.Sum256([]byte(name))
	meta := filepath.Join(dir, "contexts", "meta", hex.EncodeToString(sum[:]))
	if err := os.MkdirAll(meta, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"Name":"` + name + `","Endpoints":{"docker":{"Host":"` + host + `","SkipTLSVerify":false}}}`
	if err := os.WriteFile(filepath.Join(meta, "meta.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDockerEndpointReadsTheSelectedContext(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dir)
	t.Setenv("DOCKER_HOST", "")
	t.Setenv("DOCKER_CONTEXT", "")
	writeContext(t, dir, "odroid", "ssh://root@192.168.2.3")
	writeContext(t, dir, "colima", "unix:///Users/u/.colima/default/docker.sock")

	if got := dockerEndpoint(); got != "" {
		t.Fatalf("no configuration: got %q, want the default", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"currentContext":"odroid"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := dockerEndpoint(); got != "ssh://root@192.168.2.3" {
		t.Fatalf("current context: got %q", got)
	}
	t.Setenv("DOCKER_CONTEXT", "colima")
	if got := dockerEndpoint(); got != "unix:///Users/u/.colima/default/docker.sock" {
		t.Fatalf("DOCKER_CONTEXT: got %q", got)
	}
	t.Setenv("DOCKER_CONTEXT", "default")
	if got := dockerEndpoint(); got != "" {
		t.Fatalf("default context: got %q", got)
	}
	t.Setenv("DOCKER_HOST", "tcp://10.0.0.5:2375")
	if got := dockerEndpoint(); got != "tcp://10.0.0.5:2375" {
		t.Fatalf("DOCKER_HOST comes first: got %q", got)
	}
}

func TestEndpointOnThisMachine(t *testing.T) {
	for host, want := range map[string]bool{
		"":                               true,
		"unix:///var/run/docker.sock":    true,
		"npipe:////./pipe/docker_engine": true,
		"ssh://root@192.168.2.3":         false,
		"tcp://10.0.0.5:2375":            false,
		"tcp://127.0.0.1:2375":           true,
		"tcp://[::1]:2375":               true,
		"ssh://me@localhost":             true,
		"ssh://me@odroid.local":          false,
	} {
		if got := endpointOnThisMachine(host); got != want {
			t.Errorf("%q: got %v, want %v", host, got, want)
		}
	}
}

// An address of one of this machine's interfaces is this machine, whatever the
// scheme in front of it.
func TestAnInterfaceAddressIsThisMachine(t *testing.T) {
	local := []net.IP{net.ParseIP("192.168.35.241"), net.ParseIP("192.168.2.1")}
	for name, want := range map[string]bool{
		"192.168.2.1":    true,
		"192.168.35.241": true,
		"192.168.2.3":    false,
		"LOCALHOST":      true,
		"127.0.0.5":      true,
	} {
		if got := hostOnThisMachine(name, local); got != want {
			t.Errorf("%q: got %v, want %v", name, got, want)
		}
	}
}

// A docker command pointed at a daemon on another machine is not this machine's
// engine, so it is not listed and its containers are not read.
func TestRemoteDockerIsNotAnEngine(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("DOCKER_BIN", "")
	t.Setenv("DOCKER_CONFIG", dir)
	t.Setenv("DOCKER_CONTEXT", "")
	saved := installedDirs
	installedDirs = nil
	t.Cleanup(func() { installedDirs = saved })

	t.Setenv("DOCKER_HOST", "")
	if engineBin(DockerEngine) == "" {
		t.Fatal("a docker command speaking to this machine's daemon is not found")
	}
	t.Setenv("DOCKER_HOST", "ssh://root@192.168.2.3")
	if got := engineBin(DockerEngine); got != "" {
		t.Fatalf("a docker command speaking to another machine is an engine: %s", got)
	}
	// A test's own program is used as it is.
	t.Setenv("DOCKER_BIN", "/x/docker")
	if got := engineBin(DockerEngine); got != "/x/docker" {
		t.Fatalf("DOCKER_BIN: got %q", got)
	}
}

func TestDefaultRouteIn(t *testing.T) {
	table := `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
docker0	000011AC	00000000	0001	0	0	0	0000FFFF	0	0	0
eth0	00000000	0102A8C0	0003	0	0	0	00000000	0	0	0
eth0	0002A8C0	00000000	0001	0	0	0	00FFFFFF	0	0	0
`
	if got := defaultRouteIn(bufio.NewScanner(strings.NewReader(table))); got != "eth0" {
		t.Fatalf("got %q, want eth0", got)
	}
	noDefault := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"docker0\t000011AC\t00000000\t0001\t0\t0\t0\t0000FFFF\t0\t0\t0\n"
	if got := defaultRouteIn(bufio.NewScanner(strings.NewReader(noDefault))); got != "" {
		t.Fatalf("got %q, want none", got)
	}
}

// On a machine that is not macOS install creates the authority, asks for no
// administrator rights when no firewall is asked for, and does nothing the
// second time.
func TestEnsureInstalledOnAMachineThatIsNotMacOS(t *testing.T) {
	saved := hostSetsUpNames
	hostSetsUpNames = false
	t.Cleanup(func() { hostSetsUpNames = saved })
	useFirewallDirs(t)
	useHostsFile(t, "127.0.0.1\tlocalhost\n")
	savedInstances := hostsInstances
	hostsInstances = func() ([]ServiceInstance, error) { return nil, nil }
	t.Cleanup(func() { hostsInstances = savedInstances })

	dir := t.TempDir()
	r := &Runtime{Machine: NewMachine(dir), Elevate: func(string, []string) error {
		t.Fatal("administrator rights were asked for")
		return nil
	}}
	if err := r.EnsureInstalled(); err != nil {
		t.Fatal(err)
	}
	ca, err := os.ReadFile(filepath.Join(dir, "ca.crt"))
	if err != nil {
		t.Fatalf("no authority was created: %v", err)
	}
	if err := r.EnsureInstalled(); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(filepath.Join(dir, "ca.crt"))
	if string(again) != string(ca) {
		t.Fatal("the second install replaced the authority")
	}
}
