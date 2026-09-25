package stack

import (
	"strings"
	"testing"
)

// A command that starts a container and republishes the routes returns only
// after the running proxy opens a connection to it, so the first request after
// the command answers from the container. One request is sent, and it is not
// repeated.
func TestTheFirstRequestToANewContainerIsAnswered(t *testing.T) {
	requireE2E(t)

	m := NewMachine(t.TempDir())
	running := &Service{
		Name: "running", ContainerName: "e2efirst-running", Image: e2eImage,
		Command: e2eServer("running"), Domains: []string{"running.first.test"}, Port: 80, Network: ProxyNetwork,
	}
	started := &Service{
		Name: "started", ContainerName: "e2efirst-started", Image: e2eImage,
		Command: e2eServer("started"), Domains: []string{"started.first.test"}, Port: 80, Network: ProxyNetwork,
	}
	t.Cleanup(func() { Remove(running.ContainerName); Remove(started.ContainerName); stopEveryProxy() })

	mustRegister(t, m, GroupRef{Name: "e2efirst", Domains: []string{"first.test"}})
	if err := StartService("e2efirst", running); err != nil {
		t.Fatal(err)
	}
	waitRunning(t, running.ContainerName)
	if _, err := syncProxy(m, []string{running.ContainerName}, nil); err != nil {
		t.Fatal(err)
	}
	proxy := waitRunning(t, ProxyName)
	client := caClient(t, m, proxy.IPv4)

	if err := StartService("e2efirst", started); err != nil {
		t.Fatal(err)
	}
	waitRunning(t, started.ContainerName)
	if _, err := syncProxy(m, []string{started.ContainerName}, nil); err != nil {
		t.Fatal(err)
	}
	if got := get(t, client, "https://started.first.test/"); !strings.HasPrefix(got, "started ") {
		dumpProxy(t)
		t.Fatalf("the first request to the new container returned %q", got)
	}
}
