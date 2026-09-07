package stack

import (
	"os"
	"path/filepath"
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
	if s.Domain != "web.test" {
		t.Errorf("service domain = %q, want web.test", s.Domain)
	}
	if s.ContainerName != "proj-web" {
		t.Errorf("container name = %q, want proj-web", s.ContainerName)
	}
	if s.Port != 80 || s.Network != ProxyNetwork {
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

func TestExtraDomains(t *testing.T) {
	cfg, err := Load(write(t, `x-bay:
  domain: test
  extra_domains: [Lab.Internal]
services:
  a:
    image: nginx
  b:
    image: nginx
    labels:
      bay.domain: b.lab.internal
`))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Domains(); len(got) != 2 || got[1] != "lab.internal" {
		t.Fatalf("Domains() = %v", got)
	}
	if got := cfg.Services["b"].Domain; got != "b.lab.internal" {
		t.Errorf("b domain = %q", got)
	}
}

func TestRejectsDomainOutsideGroup(t *testing.T) {
	_, err := Load(write(t, "services:\n  a:\n    image: nginx\n    labels:\n      bay.domain: a.example.com\n"))
	if err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("err = %v, want a domain-scope error", err)
	}
}

func TestRejectsDuplicateDomainWithinGroup(t *testing.T) {
	_, err := Load(write(t, `services:
  a:
    image: nginx
    labels: {bay.domain: shared.test}
  b:
    image: nginx
    labels: {bay.domain: shared.test}
`))
	if err == nil || !strings.Contains(err.Error(), "both claim") {
		t.Fatalf("err = %v, want a duplicate-domain error", err)
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

// A database or a worker has no business claiming a domain, and issuing it a
// certificate for a port nothing serves would be worse than useless.
func TestInternalServicesGetNoDomain(t *testing.T) {
	cfg, err := Load(write(t, `name: app
services:
  web:
    image: nginx
  db:
    image: postgres
    expose: ["5432"]
    labels:
      bay.internal: "true"
`))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Services["db"].Domain; got != "" {
		t.Errorf("internal service got domain %q", got)
	}
	if got := cfg.Services["db"].Port; got != 5432 {
		t.Errorf("internal service port = %d", got)
	}
	if got := cfg.Services["db"].ContainerName; got != "app-db" {
		t.Errorf("internal service container = %q", got)
	}
	if got := cfg.Services["db"].Network; got != ProxyNetwork {
		t.Errorf("internal service network = %q, want the shared one", got)
	}
	if got := cfg.Services["web"].Domain; got != "web.test" {
		t.Errorf("routed service domain = %q", got)
	}
}

func TestInternalServiceCannotClaimADomain(t *testing.T) {
	_, err := Load(write(t, "services:\n  db:\n    image: postgres\n    labels:\n      bay.internal: \"true\"\n      bay.domain: db.test\n"))
	if err == nil || !strings.Contains(err.Error(), "internal") {
		t.Fatalf("err = %v, want a refusal", err)
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
  db:
    image: postgres:18
    expose: ["5432/tcp"]
    environment:
      - POSTGRES_PASSWORD=secret
    labels:
      - bay.internal=true
`))
	if err != nil {
		t.Fatal(err)
	}

	web := cfg.Services["web"]
	// The container port comes from the published port's container side.
	if web.Port != 3000 {
		t.Errorf("web port = %d, want 3000 from \"8080:3000\"", web.Port)
	}
	if web.Domain != "web.test" {
		t.Errorf("web domain = %q", web.Domain)
	}
	if web.Env["DATABASE_URL"] == "" || web.Env["DEBUG"] != "1" {
		t.Errorf("web environment = %v", web.Env)
	}
	if len(web.Volumes) != 1 || web.Volumes[0] != filepath.Join(filepath.Dir(cfg.Path()), "src")+":/app/src" {
		t.Errorf("web volumes = %v", web.Volumes)
	}

	// Labels in list form work the same as in mapping form.
	db := cfg.Services["db"]
	if !db.Internal || db.Domain != "" {
		t.Errorf("db = %+v, want internal with no domain", db)
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
