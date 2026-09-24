package stack

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// This fixture never registers a project or changes the machine proxy/DNS.
func TestServiceLifecycleActualRuntime(t *testing.T) {
	if os.Getenv("BAY_SERVICE_E2E") != "1" {
		t.Skip("set BAY_SERVICE_E2E=1 for isolated service containers")
	}
	before, err := List()
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("ctl-lifecycle-%d", time.Now().UnixNano())
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "private"), []byte("fixture-only"), 0600); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`name: %s
services:
  db:
    image: docker.io/library/node:26.8.1-trixie-slim
    command: [node, -e, "require('net').createServer().listen(4321,'127.0.0.1')"]
    healthcheck:
      test: [CMD, node, -e, "const c=require('net').connect(4321,'127.0.0.1',()=>{c.end();process.exit(0)});c.on('error',()=>process.exit(1))"]
      interval: 20ms
      timeout: 2s
      retries: 10
  initialize:
    image: docker.io/library/node:26.8.1-trixie-slim
    user: '%d:%d'
    read_only: true
    cap_drop: [ALL]
    volumes: ['%s:/data']
    command: [node, -e, "const f=require('fs');if(f.readFileSync('/data/private','utf8')!=='fixture-only')process.exit(1);f.appendFileSync('/data/runs','1')"]
    depends_on: {db: {condition: service_healthy}}
  web:
    image: docker.io/library/node:26.8.1-trixie-slim
    user: '%d:%d'
    read_only: true
    cap_drop: [ALL]
    command: [node, -e, "const f=require('fs');let ro=false;try{f.writeFileSync('/tmp/bay-readonly-test','x')}catch(e){ro=e.code==='EROFS'}require('http').createServer((q,s)=>s.end(JSON.stringify({uid:process.getuid(),gid:process.getgid(),ro,cap:f.readFileSync('/proc/self/status','utf8').match(/CapEff:\\s*(\\w+)/)[1]}))).listen(8080)"]
    depends_on: {initialize: {condition: service_completed_successfully}}
`, name, os.Getuid(), os.Getgid(), dir, os.Getuid(), os.Getgid())
	path := filepath.Join(dir, "compose.yaml")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	e := newServiceEngine(filepath.Join(dir, "state"))
	t.Cleanup(func() {
		for _, s := range cfg.Sorted() {
			in, ok, err := Lookup(s.ContainerName)
			if err != nil {
				t.Error(err)
				continue
			}
			if ok {
				if err = owns(cfg.Name, s, in); err != nil {
					t.Error(err)
					continue
				}
				if err = Remove(s.ContainerName); err != nil {
					t.Error(err)
				}
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
	if err := e.start(cfg, cfg.Sorted(), false); err != nil {
		t.Fatal(err)
	}
	db, _, err := Lookup(cfg.Services["db"].ContainerName)
	if err != nil {
		t.Fatal(err)
	}
	initialized, _, err := Lookup(cfg.Services["initialize"].ContainerName)
	if err != nil {
		t.Fatal(err)
	}
	web, _, err := Lookup(cfg.Services["web"].ContainerName)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 2 * time.Second}
	var response *http.Response
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		response, err = client.Get("http://" + web.IPv4 + ":8080/")
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		UID, GID int
		RO       bool
		Cap      string
	}
	if json.Unmarshal(data, &got) != nil || got.UID != os.Getuid() || got.GID != os.Getgid() || !got.RO || strings.Trim(got.Cap, "0") != "" {
		t.Fatalf("runtime restrictions not applied: %s", data)
	}
	if err := e.start(cfg, cfg.Sorted(), false); err != nil {
		t.Fatal(err)
	}
	for _, old := range []Instance{db, initialized, web} {
		current, _, err := Lookup(old.Name)
		if err != nil || !reflect.DeepEqual(old, current) {
			t.Fatalf("unchanged up replaced %s: %v", old.Name, err)
		}
	}
	runs, err := os.ReadFile(filepath.Join(dir, "runs"))
	if err != nil || string(runs) != "1" {
		t.Fatal("initializer did not run exactly once")
	}
	// A different project cannot adopt even one explicitly named owned container.
	if err := e.preflight("foreign-project", []*Service{cfg.Services["db"]}); err == nil {
		t.Fatal("foreign adoption accepted")
	}
	if err := e.start(cfg, []*Service{cfg.Services["db"]}, true); err != nil {
		t.Fatal(err)
	}
	if err := e.start(cfg, cfg.Sorted(), false); err != nil {
		t.Fatal(err)
	}
	runs, err = os.ReadFile(filepath.Join(dir, "runs"))
	if err != nil || string(runs) != "11" {
		t.Fatal("recreated database reused stale initialization proof")
	}
	// A real restart changes identity; stored success must no longer authorize web.
	// The runtime exposes timestamps only to the second. Exercise a restart
	// with a distinct exposed timestamp; same-second changes are not provable.
	current, ok, err := Lookup(initialized.Name)
	if err != nil || !ok {
		t.Fatal("initializer disappeared before external restart")
	}
	started, err := time.Parse(time.RFC3339, current.Started)
	if err != nil {
		t.Fatal(err)
	}
	if remaining := time.Until(started.Add(1100 * time.Millisecond)); remaining > 0 {
		time.Sleep(remaining)
	}
	if _, err := e.command(context.Background(), "start", "--attach", initialized.Name); err != nil {
		t.Fatal(err)
	}
	if err := e.start(cfg, cfg.Sorted(), false); err == nil {
		t.Fatal("external initializer restart retained successful completion authority")
	}
	if err := e.start(cfg, []*Service{cfg.Services["initialize"]}, true); err != nil {
		t.Fatal(err)
	}
	cfg.Services["initialize"].Command = []string{"node", "-e", "process.exit(7)"}
	if err := e.start(cfg, cfg.Sorted(), false); err == nil {
		t.Fatal("actual initializer exit7 accepted")
	}
	t.Log("verified UID/GID, read-only root, empty capabilities, 0600 mount, health gate, unchanged reuse, database replacement, exit7 and external restart refusal; preexisting containers/proxy untouched")
}
