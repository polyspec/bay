package stack

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// dockerInspect is the output `docker inspect` returns for a container. The
// name has a leading slash, the labels are a map, the image is an identifier,
// and the networks are a map keyed by network name.
const dockerInspect = `[
  {
    "Id": "9f3c1b",
    "Created": "2026-09-09T09:00:00.000000000Z",
    "Name": "/shop-web",
    "Image": "sha256:1111111111111111111111111111111111111111111111111111111111111111",
    "State": {"Status": "running", "Running": true, "StartedAt": "2026-09-09T09:00:01.000000000Z"},
    "Config": {"Labels": {"bay.role": "service", "bay.domain": "shop.test"}},
    "NetworkSettings": {"Networks": {
      "bay": {"IPAddress": "172.20.0.3", "Gateway": "172.20.0.1"}
    }},
    "Mounts": [{"Type": "bind", "Source": "/state/conf", "Destination": "/etc/nginx/conf.d", "RW": false}]
  },
  {
    "Id": "44ab02",
    "Created": "2026-09-09T08:00:00.000000000Z",
    "Name": "/shop-db",
    "Image": "sha256:2222222222222222222222222222222222222222222222222222222222222222",
    "State": {"Status": "exited", "Running": false, "StartedAt": "2026-09-09T08:00:01.000000000Z"},
    "Config": {"Labels": {"bay.role": "service"}},
    "NetworkSettings": {"Networks": {}}
  }
]`

func TestDockerContainersAreDecoded(t *testing.T) {
	list, err := decodeDockerInstances([]byte(dockerInspect))
	if err != nil {
		t.Fatalf("decoding docker containers: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("decoded %d containers, want 2", len(list))
	}
	web := list[0]
	if web.Name != "shop-web" {
		t.Errorf("name kept docker's leading slash: %q", web.Name)
	}
	if web.State != "running" {
		t.Errorf("state is %q, want running", web.State)
	}
	if web.IPv4 != "172.20.0.3" || web.Gateway != "172.20.0.1" {
		t.Errorf("address is %q gateway %q, want 172.20.0.3 and 172.20.0.1", web.IPv4, web.Gateway)
	}
	if web.Labels["bay.domain"] != "shop.test" {
		t.Errorf("labels are %v, want the domain label", web.Labels)
	}
	if web.ImageDigest == "" {
		t.Error("image digest is empty")
	}
	if web.Engine != DockerEngine {
		t.Errorf("engine is %q, want %q", web.Engine, DockerEngine)
	}
	// A container attached to no network has no address, which is what keeps it
	// out of the routes rather than an error.
	if list[1].IPv4 != "" {
		t.Errorf("a container on no network reported the address %q", list[1].IPv4)
	}
}

// A container can be attached to several networks. The address returned must
// not depend on Go's map iteration order.
func TestADockerContainerOnSeveralNetworksPicksTheSameOneEveryTime(t *testing.T) {
	const many = `[{"Id":"1","Name":"/many","State":{"Status":"running"},
	  "Config":{"Labels":{}},
	  "NetworkSettings":{"Networks":{
	    "zeta":{"IPAddress":"172.30.0.9","Gateway":"172.30.0.1"},
	    "alpha":{"IPAddress":"172.20.0.4","Gateway":"172.20.0.1"},
	    "middle":{"IPAddress":"172.25.0.5","Gateway":"172.25.0.1"}}}}]`
	for i := 0; i < 20; i++ {
		list, err := decodeDockerInstances([]byte(many))
		if err != nil {
			t.Fatal(err)
		}
		if list[0].IPv4 != "172.20.0.4" {
			t.Fatalf("run %d chose %q, want the first network by name", i, list[0].IPv4)
		}
	}
}

func TestAppleContainersCarryTheirEngine(t *testing.T) {
	const apple = `[{"configuration":{"id":"edge","labels":{"bay.role":"proxy"},
	  "image":{"descriptor":{"digest":"sha256:abc"}}},
	  "status":{"state":"running","networks":[{"ipv4Address":"192.168.64.61/24","ipv4Gateway":"192.168.64.1"}]}}]`
	list, err := decodeInstances([]byte(apple))
	if err != nil {
		t.Fatalf("decoding apple containers: %v", err)
	}
	if list[0].IPv4 != "192.168.64.61" {
		t.Errorf("address is %q, want the prefix length removed", list[0].IPv4)
	}
	if list[0].Engine != AppleEngine {
		t.Errorf("engine is %q, want %q", list[0].Engine, AppleEngine)
	}
}

// An engine that is absent, or whose daemon does not respond, returns no
// containers. A failure from one engine must not hide another engine's
// containers.
func TestAnEngineThatDoesNotAnswerContributesNothing(t *testing.T) {
	answering := engineReader{name: AppleEngine, read: func() ([]Instance, error) {
		return []Instance{{Name: "edge", Engine: AppleEngine}}, nil
	}}
	silent := engineReader{name: DockerEngine, read: func() ([]Instance, error) {
		return nil, errors.New("cannot connect to the docker daemon")
	}}

	list, err := listFrom([]engineReader{silent, answering})
	if err != nil {
		t.Fatalf("a silent engine made the list fail: %v", err)
	}
	if len(list) != 1 || list[0].Name != "edge" {
		t.Fatalf("list is %v, want only the answering engine's container", list)
	}
}

// With no engine present, List returns an error rather than an empty list.
func TestNoEnginePresentIsReported(t *testing.T) {
	if _, err := listFrom(nil); err == nil {
		t.Fatal("a machine with no engine reported an empty list instead of an error")
	}
}

// Two containers with the same name on different engines are two containers.
// The merged list must contain both.
func TestBothEnginesContributeToOneList(t *testing.T) {
	apple := engineReader{name: AppleEngine, read: func() ([]Instance, error) {
		return []Instance{{Name: "web", Engine: AppleEngine}}, nil
	}}
	docker := engineReader{name: DockerEngine, read: func() ([]Instance, error) {
		return []Instance{{Name: "web", Engine: DockerEngine}}, nil
	}}
	list, err := listFrom([]engineReader{apple, docker})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("merged list has %d containers, want both", len(list))
	}
	if list[0].Engine == list[1].Engine {
		t.Errorf("both entries came from %q", list[0].Engine)
	}
}

// A container is reachable only from its own engine's proxy, so each proxy
// receives only the routes of its own engine.
func TestRoutesNameOnlyTheContainersTheProxyCanReach(t *testing.T) {
	instances := []ServiceInstance{
		{Container: "shop-web", Group: "shop", Domains: []string{"shop.test"}, Port: 80,
			Scheme: "http", State: "running", IPv4: "192.168.64.9", Engine: AppleEngine},
		{Container: "blog-web", Group: "blog", Domains: []string{"blog.test"}, Port: 80,
			Scheme: "http", State: "running", IPv4: "172.20.0.4", Engine: DockerEngine},
	}
	routes, conflicts := routesFrom(instances)
	if len(conflicts) != 0 {
		t.Fatalf("two domains reported a conflict: %v", conflicts)
	}
	if len(routes) != 2 {
		t.Fatalf("routes are %v, want one per engine", routes)
	}
	if got := RoutesOn(routes, AppleEngine); len(got) != 1 || got[0].Domain != "shop.test" {
		t.Fatalf("apple proxy got %v, want only its own container", got)
	}
	if got := RoutesOn(routes, DockerEngine); len(got) != 1 || got[0].Domain != "blog.test" {
		t.Fatalf("docker proxy got %v, want only its own container", got)
	}
}

// One domain is claimed once per machine. Two containers on different engines
// claiming one domain produce one conflict, and the claim is resolved before
// the routes are split by engine.
func TestOneDomainIsClaimedOncePerMachineAcrossEngines(t *testing.T) {
	instances := []ServiceInstance{
		{Container: "blog-web", Group: "blog", Domains: []string{"shop.test"}, Port: 80,
			Scheme: "http", State: "running", IPv4: "172.20.0.4", Engine: DockerEngine},
		{Container: "shop-web", Group: "shop", Domains: []string{"shop.test"}, Port: 80,
			Scheme: "http", State: "running", IPv4: "192.168.64.9", Engine: AppleEngine},
	}
	routes, conflicts := routesFrom(instances)
	if len(conflicts) != 1 || conflicts[0].Domain != "shop.test" {
		t.Fatalf("conflicts are %v, want one for shop.test", conflicts)
	}
	if conflicts[0].Kept != "blog-web" || conflicts[0].Dropped != "shop-web" {
		t.Errorf("conflict kept %q and dropped %q, want the first in order kept",
			conflicts[0].Kept, conflicts[0].Dropped)
	}
	// The Docker container holds the domain, so the Apple proxy has no route.
	if got := RoutesOn(routes, AppleEngine); len(got) != 0 {
		t.Fatalf("apple proxy got %v, want none: the domain is held on the other engine", got)
	}
	if got := RoutesOn(routes, DockerEngine); len(got) != 1 {
		t.Fatalf("docker proxy got %v, want the container that holds the domain", got)
	}
}

// A daemon that has stopped answering accepts the connection and never replies.
// Without a bound the command never returns and everything that reads the
// container list stops with it, which is how a whole test run came to sit for
// ten minutes on one call. A command that does not answer within its bound is
// an engine that does not answer, which contributes nothing.
func TestAnEngineThatNeverAnswersIsGivenUpOn(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "docker")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nsleep 300\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCKER_BIN", fake)

	start := time.Now()
	_, err := runEngineWithin(300*time.Millisecond, DockerEngine, "container", "ls")
	if err == nil {
		t.Fatal("a command that never answered was reported as having answered")
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("the command was waited on for %s, so it is not bound", took)
	}
	if !strings.Contains(err.Error(), "no answer within") {
		t.Errorf("the error does not say the engine did not answer: %v", err)
	}

	// The list treats it as an engine that does not answer: no containers, and
	// no error, so one stuck engine does not stop the machine reading another.
	stuck := engineReader{name: DockerEngine, read: func() ([]Instance, error) {
		_, err := runEngineWithin(200*time.Millisecond, DockerEngine, "container", "ls")
		return nil, err
	}}
	answering := engineReader{name: AppleEngine, read: func() ([]Instance, error) {
		return []Instance{{Name: "edge", Engine: AppleEngine}}, nil
	}}
	list, err := listFrom([]engineReader{stuck, answering})
	if err != nil {
		t.Fatalf("a stuck engine made the list fail: %v", err)
	}
	if len(list) != 1 || list[0].Name != "edge" {
		t.Fatalf("list is %v, want only the answering engine's container", list)
	}
}
