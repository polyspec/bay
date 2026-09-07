package stack

import (
	"os"
	"strings"
	"testing"
)

const editable = `# a project
name: proj

x-bay:
  domain: test          # the primary
  extra_domains:
    - lab.internal

services:
  web:
    image: nginx        # keep this comment
    expose: ["80"]
  api:
    image: nginx
    labels:
      bay.domain: api.lab.internal
`

func loadEditable(t *testing.T, body string) *Config {
	t.Helper()
	cfg, err := Load(write(t, body))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func reload(t *testing.T, cfg *Config) *Config {
	t.Helper()
	next, err := Load(cfg.Path())
	if err != nil {
		t.Fatalf("the file no longer loads: %v", err)
	}
	return next
}

func body(t *testing.T, cfg *Config) string {
	t.Helper()
	b, err := os.ReadFile(cfg.Path())
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestAddDomainKeepsComments(t *testing.T) {
	cfg := loadEditable(t, editable)
	if err := AddDomain(cfg, " Staging.Test "); err != nil {
		t.Fatal(err)
	}
	next := reload(t, cfg)
	if got := next.Domains(); len(got) != 3 || got[2] != "staging.test" {
		t.Fatalf("Domains() = %v", got)
	}
	// Editing through the node tree is only worth it if the file survives it.
	out := body(t, cfg)
	for _, want := range []string{"# a project", "# the primary", "# keep this comment"} {
		if !strings.Contains(out, want) {
			t.Errorf("comment %q was lost:\n%s", want, out)
		}
	}
}

func TestAddDomainRejectsDuplicatesAndJunk(t *testing.T) {
	cfg := loadEditable(t, editable)
	if err := AddDomain(cfg, "lab.internal"); err == nil {
		t.Error("adding an existing domain was allowed")
	}
	if err := AddDomain(cfg, "not a domain"); err == nil {
		t.Error("a name with a space was allowed")
	}
	if err := AddDomain(cfg, "-bad.test"); err == nil {
		t.Error("a label starting with - was allowed")
	}
}

func TestRemoveDomainRefusesWhenStillUsed(t *testing.T) {
	cfg := loadEditable(t, editable)
	if err := RemoveDomain(cfg, "lab.internal"); err == nil {
		t.Fatal("removed a domain a service still claims")
	} else if !strings.Contains(err.Error(), "api") {
		t.Errorf("error should name the service: %v", err)
	}
	if err := RemoveDomain(cfg, "test"); err == nil {
		t.Error("removed the primary domain")
	}
}

func TestRemoveDomainWorksOnceFree(t *testing.T) {
	cfg := loadEditable(t, strings.Replace(editable,
		"    labels:\n      bay.domain: api.lab.internal\n", "", 1))
	if err := RemoveDomain(cfg, "lab.internal"); err != nil {
		t.Fatal(err)
	}
	if got := reload(t, cfg).Domains(); len(got) != 1 || got[0] != "test" {
		t.Fatalf("Domains() = %v", got)
	}
}

// Renaming the primary domain has to carry the services that were sitting
// under it, or they end up pointing at a domain the group no longer serves.
func TestSetPrimaryDomainMovesServices(t *testing.T) {
	cfg := loadEditable(t, editable)
	if err := SetPrimaryDomain(cfg, "dev.test"); err != nil {
		t.Fatal(err)
	}
	next := reload(t, cfg)
	if next.Domain != "dev.test" {
		t.Fatalf("domain = %q", next.Domain)
	}
	if got := next.Services["web"].Domain; got != "web.dev.test" {
		t.Errorf("web domain = %q, want web.dev.test", got)
	}
	// A service pinned to another domain must not be dragged along.
	if got := next.Services["api"].Domain; got != "api.lab.internal" {
		t.Errorf("api domain = %q, want it left alone", got)
	}
}

func TestSetServiceDomain(t *testing.T) {
	cfg := loadEditable(t, editable)
	if err := SetServiceDomain(cfg, "web", "www.test"); err != nil {
		t.Fatal(err)
	}
	if got := reload(t, cfg).Services["web"].Domain; got != "www.test" {
		t.Fatalf("web domain = %q", got)
	}
}

func TestSetServiceDomainRefusesOutsideAndDuplicate(t *testing.T) {
	cfg := loadEditable(t, editable)
	if err := SetServiceDomain(cfg, "web", "web.example.com"); err == nil {
		t.Error("a domain outside the group was allowed")
	}
	if err := SetServiceDomain(cfg, "web", "api.lab.internal"); err == nil {
		t.Error("a domain another service already claims was allowed")
	}
	if err := SetServiceDomain(cfg, "nope", "x.test"); err == nil {
		t.Error("an unknown service was allowed")
	}
	// None of those should have touched the file.
	if got := reload(t, cfg).Services["web"].Domain; got != "web.test" {
		t.Fatalf("web domain changed to %q despite the errors", got)
	}
}

func TestEditLeavesNoScratchFile(t *testing.T) {
	cfg := loadEditable(t, editable)
	if err := AddDomain(cfg, "extra.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cfg.Path() + ".bay-edit"); !os.IsNotExist(err) {
		t.Fatal("the validation scratch file was left behind")
	}
}
