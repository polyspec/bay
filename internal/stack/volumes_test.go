package stack

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

func TestManagedVolumeDeclaration(t *testing.T) {
	_, err := Load(write(t, `name: project
services:
  db:
    image: postgres
    volumes: [data:/var/lib/postgresql]
volumes:
  data: {name: project-postgres, driver: local, driver_opts: {size: 32g}}
`))
	if err != nil {
		t.Fatal(err)
	}
}

func managedConfig(t *testing.T) *Config {
	t.Helper()
	cfg, err := Load(write(t, `name: project
services:
  db: {image: postgres, volumes: ['data:/data']}
volumes:
  data: {name: project-postgres, driver: local, driver_opts: {size: 32g}}
`))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}
func volumeFixture(t *testing.T) (*serviceEngine, *fakeServices, map[string]runtimeVolume, *int) {
	t.Helper()
	e, f := fakeEngine(t)
	volumes := map[string]runtimeVolume{}
	creates := new(int)
	e.command = func(ctx context.Context, args ...string) ([]byte, error) {
		if args[0] != "volume" {
			return f.command(ctx, args...)
		}
		name := args[len(args)-1]
		if args[1] == "inspect" {
			v, ok := volumes[name]
			if !ok {
				return nil, fmt.Errorf("volume not found: %s", name)
			}
			return json.Marshal([]runtimeVolume{v})
		}
		if args[1] != "create" {
			return nil, fmt.Errorf("unexpected volume mutation")
		}
		if _, ok := volumes[name]; ok {
			return nil, fmt.Errorf("volume already exists")
		}
		var v runtimeVolume
		v.Configuration.Name = name
		v.Configuration.Driver = "local"
		v.Configuration.SizeInBytes = 1 << 30
		v.Configuration.Labels = map[string]string{}
		for i := 2; i < len(args)-1; i++ {
			switch args[i] {
			case "--label":
				i++
				key, value, _ := strings.Cut(args[i], "=")
				v.Configuration.Labels[key] = value
			case "-s":
				i++
				v.Configuration.SizeInBytes, _ = strconv.ParseInt(args[i], 10, 64)
			}
		}
		volumes[name] = v
		*creates++
		return nil, nil
	}
	return e, f, volumes, creates
}

func TestManagedVolumeCreationReuseAndDownPreservation(t *testing.T) {
	cfg := managedConfig(t)
	e, _, volumes, creates := volumeFixture(t)
	if err := e.start(cfg, cfg.Sorted(), false); err != nil {
		t.Fatal(err)
	}
	if *creates != 1 || volumes["project-postgres"].Configuration.SizeInBytes != 32<<30 {
		t.Fatal("declared volume was not created with exact size")
	}
	if err := e.start(cfg, cfg.Sorted(), false); err != nil {
		t.Fatal(err)
	}
	if err := e.stop(cfg, cfg.Sorted(), true); err != nil {
		t.Fatal(err)
	}
	if err := e.start(cfg, cfg.Sorted(), false); err != nil {
		t.Fatal(err)
	}
	if *creates != 1 {
		t.Fatal("up/down replaced durable volume")
	}
	cfg.Volumes["data"].Size = 64 << 30
	if err := e.start(cfg, cfg.Sorted(), false); err == nil {
		t.Fatal("volume resize was silently accepted")
	}
}

func TestManagedVolumeOwnershipAndExternalSemantics(t *testing.T) {
	for _, mutation := range []string{"unowned", "other-project", "other-key", "size", "driver"} {
		t.Run(mutation, func(t *testing.T) {
			cfg := managedConfig(t)
			e, f, volumes, _ := volumeFixture(t)
			if err := e.prepareVolumes(cfg.Name, cfg.Sorted(), true); err != nil {
				t.Fatal(err)
			}
			v := volumes["project-postgres"]
			switch mutation {
			case "unowned":
				v.Configuration.Labels = nil
			case "other-project":
				v.Configuration.Labels[LabelGroup] = "foreign"
			case "other-key":
				v.Configuration.Labels[LabelVolume] = "other"
			case "size":
				v.Configuration.SizeInBytes++
			case "driver":
				v.Configuration.Driver = "other"
			}
			volumes["project-postgres"] = v
			if err := e.start(cfg, cfg.Sorted(), false); err == nil || len(f.events) != 0 {
				t.Fatal("foreign or changed volume was used")
			}
		})
	}
	cfg := managedConfig(t)
	e, f, volumes, creates := volumeFixture(t)
	cfg.Volumes["data"].External = true
	cfg.Volumes["data"].Size = 0
	if err := e.start(cfg, cfg.Sorted(), false); err == nil || *creates != 0 || len(f.events) != 0 {
		t.Fatal("missing external volume was created")
	}
	var existing runtimeVolume
	existing.Configuration.Name = "project-postgres"
	volumes[existing.Configuration.Name] = existing
	if err := e.start(cfg, cfg.Sorted(), false); err != nil || *creates != 0 {
		t.Fatalf("explicit external volume modified: %v", err)
	}
}

func TestManagedVolumeDeclarationRejectsUnsupportedOptions(t *testing.T) {
	for _, declaration := range []string{"{driver: nfs}", "{driver_opts: {type: nfs}}", "{driver_opts: {size: -1}}", "{driver_opts: {size: ''}}", "{driver_opts: {size: 1000000000000000000P}}", "{external: true, driver: local}", "{external: true, driver_opts: {size: 32g}}"} {
		if _, err := Load(write(t, "services:\n  web: {image: nginx}\nvolumes:\n  data: "+declaration+"\n")); err == nil {
			t.Fatalf("accepted unsupported volume %s", declaration)
		}
	}
	cfg, err := Load(write(t, "name: project\nservices:\n  web: {image: nginx, volumes: ['data:/data']}\nvolumes:\n  data: {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Volumes["data"].Name != "project-data" || cfg.Volumes["data"].Size != 0 {
		t.Fatal("managed defaults changed")
	}
}
