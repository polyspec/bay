package stack

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestLocalImageTagActualRuntime(t *testing.T) {
	if os.Getenv("BAY_SERVICE_E2E") != "1" {
		t.Skip("set BAY_SERVICE_E2E=1 for isolated local-image verification")
	}
	before, err := List()
	if err != nil {
		t.Fatal(err)
	}
	group := fmt.Sprintf("ctl-local-%d", time.Now().UnixNano())
	imageName := "localhost/" + group
	ref := imageName + ":0.0.1"
	e := newServiceEngine(t.TempDir())
	e.timeout = 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	// A unique local reference reproduces the cache semantics of local builds.
	// The application image and its references are never modified.
	if _, err := e.image(ctx, "docker.io/library/node:26.8.1-trixie-slim"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.command(ctx, "image", "tag", "docker.io/library/node:26.8.1-trixie-slim", ref); err != nil {
		t.Fatal(err)
	}
	svc := &Service{Name: "initialize", ContainerName: group + "-initialize", Image: ref,
		Command: []string{"node", "-e", "console.log('fixture-public-marker')"}, OneShot: true, Network: ProxyNetwork}
	t.Cleanup(func() {
		in, found, err := Lookup(svc.ContainerName)
		if err != nil {
			t.Error(err)
		} else if found {
			if err := owns(group, svc, in); err != nil {
				t.Error(err)
			} else if err := Remove(svc.ContainerName); err != nil {
				t.Error(err)
			}
		}
		if _, err := e.command(context.Background(), "image", "rm", ref); err != nil {
			t.Error(err)
		}
		after, err := List()
		if err != nil {
			t.Error(err)
			return
		}
		actual := map[string]Instance{}
		for _, in := range after {
			actual[in.Name] = in
		}
		for _, in := range before {
			if !reflect.DeepEqual(in, actual[in.Name]) {
				t.Errorf("preexisting container changed: %s", in.Name)
			}
		}
	})
	digest, err := e.image(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.command(ctx, "image", "inspect", imageName+"@"+digest); err == nil {
		t.Fatal("local image fixture unexpectedly has a digest alias")
	}
	if err := e.reconcile(group, svc, false); err != nil {
		t.Fatalf("valid local tag did not initialize: %v", err)
	}
	in, found, err := Lookup(svc.ContainerName)
	if err != nil || !found || in.ImageDigest != digest || in.State != "stopped" || !e.completed(in, serviceFingerprint(group, svc, digest, trustArguments(e.dir, svc))) {
		t.Fatalf("local-tag completion lacks exact image identity: %+v, %v", in, err)
	}
	logs, err := exec.CommandContext(ctx, engineBin(ServiceEngine()), "logs", svc.ContainerName).Output()
	if err != nil || !strings.Contains(string(logs), "fixture-public-marker") {
		t.Fatalf("native logs did not retain the public process marker: %v", err)
	}
	t.Log("native logs retain stdout from an attached initializer; applications must not print values forbidden in logs")
	if err := e.reconcile(group, svc, false); err != nil {
		t.Fatal(err)
	}
	after, _, err := Lookup(svc.ContainerName)
	if err != nil || !reflect.DeepEqual(in, after) {
		t.Fatal("identical local-tag initialization was repeated")
	}
	svc.Command = []string{"node", "-e", "console.error('fixture-private-value');process.exit(7)"}
	if err := e.reconcile(group, svc, false); err == nil || strings.Contains(err.Error(), "fixture-private-value") {
		t.Fatalf("initializer failure accepted or disclosed process output: %v", err)
	}
	svc.Memory = "fixture-private-value"
	if err := e.reconcile(group, svc, false); err == nil || !strings.Contains(err.Error(), "create") || strings.Contains(err.Error(), "fixture-private-value") {
		t.Fatalf("creation failure lacked its stage or disclosed runtime arguments: %v", err)
	}
}
