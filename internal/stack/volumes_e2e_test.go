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

func TestManagedVolumeActualRuntime(t *testing.T) {
	if os.Getenv("BAY_SERVICE_E2E") != "1" {
		t.Skip("set BAY_SERVICE_E2E=1 for isolated volume verification")
	}
	before, err := List()
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("ctl-volume-%d", time.Now().UnixNano())
	cfg, err := Load(write(t, fmt.Sprintf(`name: %s
services:
  store:
    image: docker.io/library/node:26.8.1-trixie-slim
    labels: {bay.internal: 'true'}
    volumes: ['data:/data']
    command: [node, -e, "require('fs').appendFileSync('/data/persist','x');setInterval(()=>{},1000)"]
    healthcheck:
      test: [CMD, node, -e, "require('fs').accessSync('/data/persist')"]
      interval: 20ms
      timeout: 2s
      retries: 10
volumes:
  data: {driver: local, driver_opts: {size: 64m}}
`, name)))
	if err != nil {
		t.Fatal(err)
	}
	e := newServiceEngine(t.TempDir())
	v := cfg.Volumes["data"]
	s := cfg.Services["store"]
	t.Cleanup(func() {
		if err := e.stop(cfg, cfg.Sorted(), true); err != nil {
			t.Error(err)
			return
		}
		volume, ok, err := e.inspectVolume(context.Background(), v.Name)
		if err != nil {
			t.Error(err)
		} else if ok {
			if err = checkVolume(cfg.Name, v, volume); err != nil {
				t.Error(err)
			} else if _, err = e.command(context.Background(), "volume", "rm", v.Name); err != nil {
				t.Error(err)
			}
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
	if err = e.start(cfg, cfg.Sorted(), false); err != nil {
		t.Fatal(err)
	}
	original, ok, err := e.inspectVolume(context.Background(), v.Name)
	if err != nil || !ok {
		t.Fatal("created volume missing")
	}
	if original.Configuration.SizeInBytes != 64<<20 {
		t.Fatal("runtime volume size differs")
	}
	if err = e.start(cfg, cfg.Sorted(), false); err != nil {
		t.Fatal(err)
	}
	read := func(want string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		// Unlike the production startup command, this explicit test read captures
		// only fixture content, never a process environment or private config.
		out, err := exec.CommandContext(ctx, containerBin(), "exec", s.ContainerName, "cat", "/data/persist").Output()
		if err != nil || strings.TrimSpace(string(out)) != want {
			t.Fatalf("persistent fixture data differs: %q %v", out, err)
		}
		if ctx.Err() != nil {
			t.Fatal(ctx.Err())
		}
	}
	read("x")
	if err = e.stop(cfg, cfg.Sorted(), true); err != nil {
		t.Fatal(err)
	}
	after, ok, err := e.inspectVolume(context.Background(), v.Name)
	if err != nil || !ok || !reflect.DeepEqual(original, after) {
		t.Fatal("down changed managed volume")
	}
	if err = e.start(cfg, cfg.Sorted(), false); err != nil {
		t.Fatal(err)
	}
	read("xx")
	if err = e.prepareVolumes("other-project", cfg.Sorted(), true); err == nil {
		t.Fatal("foreign project reused volume")
	}
	v.Size = 128 << 20
	if err = e.prepareVolumes(cfg.Name, cfg.Sorted(), true); err == nil {
		t.Fatal("existing volume silently resized")
	}
	v.Size = 64 << 20
	t.Log("created owned64MiB volume; unchanged up/down retained identity and data; foreign owner/resize rejected; fixture-only volume removed by test cleanup")
}
