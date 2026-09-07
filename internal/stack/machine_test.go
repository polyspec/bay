package stack

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestGroupRegistryRoundTrip(t *testing.T) {
	m := NewMachine(t.TempDir())

	if groups, err := m.Groups(); err != nil || len(groups) != 0 {
		t.Fatalf("Groups() on a fresh machine = %v, %v", groups, err)
	}
	// A fresh machine already delegates its default domain; projects inherit it
	// rather than each declaring one.
	if domains, err := m.Domains(); err != nil || len(domains) != 1 || domains[0] != DefaultDomain {
		t.Fatalf("Domains() on a fresh machine = %v, %v", domains, err)
	}

	if err := m.Register(GroupRef{Name: "beta", StackPath: "/b/stack.yaml", Domains: []string{"test"}}); err != nil {
		t.Fatal(err)
	}
	if err := m.Register(GroupRef{Name: "alpha", StackPath: "/a/stack.yaml", Domains: []string{"test", "lab.internal"}}); err != nil {
		t.Fatal(err)
	}

	groups, err := m.Groups()
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || groups[0].Name != "alpha" || groups[1].Name != "beta" {
		t.Fatalf("Groups() = %+v, want alpha then beta", groups)
	}
	if groups[0].UpdatedAt.IsZero() {
		t.Error("Register did not stamp UpdatedAt")
	}

	// The union is what the resolver entries have to cover, deduplicated.
	domains, err := m.Domains()
	if err != nil {
		t.Fatal(err)
	}
	// The union is sorted, so the list is stable however it was assembled.
	if len(domains) != 2 || domains[0] != "lab.internal" || domains[1] != "test" {
		t.Fatalf("Domains() = %v", domains)
	}

	// Dropping one group drops only the domains no other group claims.
	if err := m.Unregister("alpha"); err != nil {
		t.Fatal(err)
	}
	domains, err = m.Domains()
	if err != nil {
		t.Fatal(err)
	}
	if len(domains) != 1 || domains[0] != "test" {
		t.Fatalf("Domains() after unregister = %v, want [test]", domains)
	}
}

// Domains are machine state: delegated once in /etc/resolver, used by every
// project. These cover managing that list.
func TestMachineDomains(t *testing.T) {
	m := NewMachine(t.TempDir())

	if err := m.AddDomain(" Lab.Test "); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.Domains(); len(got) != 2 || got[0] != "lab.test" || got[1] != DefaultDomain {
		t.Fatalf("Domains() = %v", got)
	}
	if err := m.AddDomain("lab.test"); err == nil {
		t.Error("adding a domain twice was allowed")
	}
	if err := m.AddDomain("not a domain"); err == nil {
		t.Error("a name with a space was allowed")
	}

	// The default cannot simply be dropped; something has to take its place.
	if err := m.RemoveDomain(DefaultDomain); err == nil {
		t.Error("removed the default domain")
	}
	if err := m.SetDefaultDomain("lab.test"); err != nil {
		t.Fatal(err)
	}
	s, err := m.Settings()
	if err != nil {
		t.Fatal(err)
	}
	if s.Domain != "lab.test" {
		t.Fatalf("default = %q", s.Domain)
	}
	// The former default stays delegated rather than disappearing.
	if got, _ := m.Domains(); len(got) != 2 {
		t.Fatalf("Domains() after changing the default = %v", got)
	}
	if err := m.RemoveDomain(DefaultDomain); err != nil {
		t.Fatalf("the former default should now be removable: %v", err)
	}
}

// A domain a project pinned in its Compose file belongs to that file, and
// removing it from the machine would leave the project pointing at nothing.
func TestMachineWillNotRemoveAPinnedDomain(t *testing.T) {
	m := NewMachine(t.TempDir())
	if err := m.AddDomain("pinned.test"); err != nil {
		t.Fatal(err)
	}
	if err := m.Register(GroupRef{Name: "shop", Domains: []string{"pinned.test"}}); err != nil {
		t.Fatal(err)
	}
	err := m.RemoveDomain("pinned.test")
	if err == nil || !strings.Contains(err.Error(), "shop") {
		t.Fatalf("err = %v, want a refusal naming the project", err)
	}
}

func TestRegisterReplacesSameName(t *testing.T) {
	m := NewMachine(t.TempDir())
	if err := m.Register(GroupRef{Name: "g", Domains: []string{"old.test"}}); err != nil {
		t.Fatal(err)
	}
	if err := m.Register(GroupRef{Name: "g", Domains: []string{"new.test"}}); err != nil {
		t.Fatal(err)
	}
	groups, err := m.Groups()
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(groups[0].Domains) != 1 || groups[0].Domains[0] != "new.test" {
		t.Fatalf("Groups() = %+v, want one entry carrying new.test", groups)
	}
}

func TestMachinePaths(t *testing.T) {
	m := NewMachine("/tmp/state")
	for name, got := range map[string]string{
		"certs":  m.CertDir(),
		"conf.d": m.ConfDir(),
		"logs":   m.LogDir(),
	} {
		if want := filepath.Join("/tmp/state", name); got != want {
			t.Errorf("%s dir = %q, want %q", name, got, want)
		}
	}
}
