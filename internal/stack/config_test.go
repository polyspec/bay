package stack

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// write puts a Compose file in a directory named after the test, so the project
// name defaulting from the directory is exercised too.
func write(t *testing.T, body string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "proj")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "compose.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDefaults(t *testing.T) {
	cfg, err := Load(write(t, "services:\n  web:\n    image: nginx\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Name != "proj" {
		t.Errorf("group name = %q, want the stack file's directory", cfg.Name)
	}
	if got := cfg.Domains(); len(got) != 1 || got[0] != "test" {
		t.Fatalf("Domains() = %v, want [test]", got)
	}
	s := cfg.Services["web"]
	if len(s.Domains) != 0 || !s.Internal() {
		t.Errorf("service without x-bay.domains: domains %v, want none and internal", s.Domains)
	}
	if s.ContainerName != "proj-web" {
		t.Errorf("container name = %q, want proj-web", s.ContainerName)
	}
	// No port is declared, so the service has none.
	if s.Port != 0 || s.Network != ProxyNetwork {
		t.Errorf("port/network = %d/%q", s.Port, s.Network)
	}
}

// Two groups may use the same service names; only the container names have to
// differ, which is what the group prefix is for.
func TestContainerNamesAreGroupScoped(t *testing.T) {
	body := "name: %s\nservices:\n  app:\n    image: nginx\n"
	a, err := Load(write(t, strings.Replace(body, "%s", "alpha", 1)))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Load(write(t, strings.Replace(body, "%s", "beta", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if a.Services["app"].ContainerName == b.Services["app"].ContainerName {
		t.Fatalf("both groups produced %q", a.Services["app"].ContainerName)
	}
}

func TestServiceDomains(t *testing.T) {
	path := write(t, `x-bay:
  domain: site.test
  extra_domains: [Lab.Internal]
services:
  web:
    image: nginx
    expose: ["80"]
    x-bay:
      domains:
        - console.site.test
        - Example.Site.Test
        - ${ADMIN_HOST}
        - web.lab.internal
`)
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), ".env"), []byte("ADMIN_HOST=admin.example.site.test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Domains(); len(got) != 2 || got[1] != "lab.internal" {
		t.Fatalf("Domains() = %v", got)
	}
	web := cfg.Services["web"]
	want := []string{"console.site.test", "example.site.test", "admin.example.site.test", "web.lab.internal"}
	if !reflect.DeepEqual(web.Domains, want) || web.Internal() {
		t.Fatalf("web domains = %v, internal %v; want %v", web.Domains, web.Internal(), want)
	}
}

func TestServiceDomainsRejected(t *testing.T) {
	cases := map[string]struct{ service, want string }{
		"outside the project": {
			"x-bay:\n      domains: [a.example.com]", "outside"},
		"empty list": {
			"x-bay:\n      domains: []", "empty"},
		"not a list": {
			"x-bay:\n      domains: a.site.test", "must be a list"},
		"duplicate": {
			"x-bay:\n      domains: [a.site.test, A.site.test]", "repeats"},
		"invalid name": {
			"x-bay:\n      domains: [a_b.site.test]", "may only contain"},
		"empty name": {
			"x-bay:\n      domains: ['']", "empty"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Load(write(t, "x-bay:\n  domain: site.test\nservices:\n  web:\n    image: nginx\n    "+c.service+"\n"))
			if err == nil || !strings.Contains(err.Error(), c.want) || !strings.Contains(err.Error(), `"web"`) {
				t.Fatalf("err = %v, want it to name the service and contain %q", err, c.want)
			}
		})
	}
}

func TestRejectsDuplicateDomainWithinGroup(t *testing.T) {
	_, err := Load(write(t, `services:
  a:
    image: nginx
    expose: ["80"]
    x-bay: {domains: [shared.test]}
  b:
    image: nginx
    expose: ["80"]
    x-bay: {domains: [b.test, shared.test]}
`))
	if err == nil || !strings.Contains(err.Error(), "both claim shared.test") ||
		!strings.Contains(err.Error(), `"a"`) || !strings.Contains(err.Error(), `"b"`) {
		t.Fatalf("err = %v, want a duplicate-domain error naming both services", err)
	}
}

func TestRejectsBadNames(t *testing.T) {
	if _, err := Load(write(t, "name: 'has space'\nservices:\n  a:\n    image: nginx\n")); err == nil {
		t.Error("expected an error for a group name with a space")
	}
	if _, err := Load(write(t, "services:\n  a:\n    image: nginx\n  'b c':\n    image: nginx\n")); err == nil {
		t.Error("expected an error for a service name with a space")
	}
	if _, err := Load(write(t, "services:\n  a: {}\n")); err == nil {
		t.Error("expected an error for a service without an image")
	}
}

func TestRefCarriesDomains(t *testing.T) {
	cfg, err := Load(write(t, "name: g\nx-bay:\n  domain: test\n  extra_domains: [lab.internal]\nservices:\n  a:\n    image: nginx\n"))
	if err != nil {
		t.Fatal(err)
	}
	ref := cfg.Ref()
	if ref.Name != "g" || len(ref.Domains) != 2 {
		t.Fatalf("Ref() = %+v", ref)
	}
	if ref.StackPath != cfg.Path() {
		t.Errorf("Ref().StackPath = %q, want %q", ref.StackPath, cfg.Path())
	}
}

// A database or a worker declares no domains: it runs on the shared network and
// receives no route and no certificate.
func TestInternalServicesGetNoDomain(t *testing.T) {
	cfg, err := Load(write(t, `name: app
services:
  web:
    image: nginx
    expose: ["80"]
    x-bay:
      domains: [web.test]
  db:
    image: postgres
    expose: ["5432"]
`))
	if err != nil {
		t.Fatal(err)
	}
	db := cfg.Services["db"]
	if !db.Internal() || len(db.Domains) != 0 {
		t.Errorf("db domains = %v, want internal", db.Domains)
	}
	if db.Port != 5432 {
		t.Errorf("internal service port = %d", db.Port)
	}
	if db.ContainerName != "app-db" {
		t.Errorf("internal service container = %q", db.ContainerName)
	}
	if db.Network != ProxyNetwork {
		t.Errorf("internal service network = %q, want the shared one", db.Network)
	}
	if got := cfg.Services["web"].Domains; !reflect.DeepEqual(got, []string{"web.test"}) {
		t.Errorf("routed service domains = %v", got)
	}
}

// A Compose file written for other tools carries plenty this one has no
// opinion about. Ignoring the rest rather than rejecting it is what makes the
// file usable by `docker compose` as well.
func TestIgnoresTheRestOfCompose(t *testing.T) {
	cfg, err := Load(write(t, `name: shop
x-bay:
  domain: test
services:
  web:
    image: node:22
    build: .
    restart: unless-stopped
    depends_on: [db]
    healthcheck:
      test: ["CMD", "true"]
    ports:
      - "8080:3000"
    environment:
      DATABASE_URL: postgres://db:5432/app
      DEBUG: "1"
    volumes:
      - ./src:/app/src
    x-bay:
      domains: [web.test]
  db:
    image: postgres:18
    expose: ["5432/tcp"]
    environment:
      - POSTGRES_PASSWORD=secret
    labels:
      - com.example.owner=shop
`))
	if err != nil {
		t.Fatal(err)
	}

	web := cfg.Services["web"]
	// The container port comes from the published port's container side.
	if web.Port != 3000 {
		t.Errorf("web port = %d, want 3000 from \"8080:3000\"", web.Port)
	}
	if !reflect.DeepEqual(web.Domains, []string{"web.test"}) {
		t.Errorf("web domains = %v", web.Domains)
	}
	if web.Env["DATABASE_URL"] == "" || web.Env["DEBUG"] != "1" {
		t.Errorf("web environment = %v", web.Env)
	}
	if len(web.Volumes) != 1 || web.Volumes[0] != filepath.Join(filepath.Dir(cfg.Path()), "src")+":/app/src" {
		t.Errorf("web volumes = %v", web.Volumes)
	}

	// Labels in list form are read the same as in mapping form.
	db := cfg.Services["db"]
	if !db.Internal() {
		t.Errorf("db = %+v, want internal", db)
	}
	if db.Port != 5432 {
		t.Errorf("db port = %d, want 5432 from expose", db.Port)
	}
	if db.Env["POSTGRES_PASSWORD"] != "secret" {
		t.Errorf("db environment = %v", db.Env)
	}
}

func TestCommandAcceptsBothSpellings(t *testing.T) {
	cfg, err := Load(write(t, `services:
  a:
    image: nginx
    command: npm run dev
  b:
    image: nginx
    command: ["npm", "run", "dev"]
  c:
    image: nginx
    entrypoint: ["/bin/sh", "-c"]
    command: ["echo hi"]
`))
	if err != nil {
		t.Fatal(err)
	}
	want := "npm run dev"
	if got := strings.Join(cfg.Services["a"].Command, " "); got != want {
		t.Errorf("a command = %q", got)
	}
	if got := strings.Join(cfg.Services["b"].Command, " "); got != want {
		t.Errorf("b command = %q", got)
	}
	// An entrypoint comes first, the way a container runs it.
	if got := strings.Join(cfg.Services["c"].Command, " "); got != "/bin/sh -c echo hi" {
		t.Errorf("c command = %q", got)
	}
}

func TestFindsTheComposeFileInADirectory(t *testing.T) {
	dir := filepath.Dir(write(t, "services:\n  a:\n    image: nginx\n"))
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(cfg.Path()) != "compose.yaml" {
		t.Errorf("found %q", cfg.Path())
	}
	if _, err := Load(t.TempDir()); err == nil {
		t.Error("a directory with no Compose file was accepted")
	}
}
