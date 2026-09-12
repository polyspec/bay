package stack

import (
	"os"
	"path/filepath"
	"testing"
)

// Delegation is added by the commands that serve a domain and withdrawn only by
// the commands that are asked to withdraw it. These cover which privileged
// steps each of those produces.

func resolverFixture(t *testing.T, addr string, domains ...string) string {
	t.Helper()
	dir := useTempResolverDir(t)
	in := Install{Addr: addr}
	for _, d := range domains {
		body := in.resolverBody(d)
		if err := os.WriteFile(filepath.Join(dir, d), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// A machine serving a domain that is already delegated asks for nothing. This
// is the ordinary case: every project under an existing domain runs with no
// password prompt.
func TestServingADelegatedDomainNeedsNoPrivilegedStep(t *testing.T) {
	resolverFixture(t, "127.0.0.1:5354", "test")
	in := Install{Domains: []string{"test"}, Addr: "127.0.0.1:5354"}
	if steps := in.privilegedSteps(); len(steps) != 0 {
		t.Errorf("serving a delegated domain asked for %v", steps)
	}
}

// Another state directory's domains are none of this command's business. The
// entries for them are left alone, so a machine with two state directories does
// not lose a delegation to whichever command ran last.
func TestDomainsThisCommandDoesNotServeAreLeftAlone(t *testing.T) {
	dir := resolverFixture(t, "127.0.0.1:5354", "test", "devel", "staging")
	in := Install{Domains: []string{"test"}, Addr: "127.0.0.1:5354"}
	if steps := in.privilegedSteps(); len(steps) != 0 {
		t.Fatalf("serving one domain asked to change others: %v", steps)
	}
	if err := in.applyAsRoot(); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"test", "devel", "staging"} {
		if _, err := os.Stat(filepath.Join(dir, d)); err != nil {
			t.Errorf("%s was removed by a command that does not serve it", d)
		}
	}
}

// Withdrawing is asked for by name, never inferred.
func TestRetiringADomainRemovesItsEntry(t *testing.T) {
	dir := resolverFixture(t, "127.0.0.1:5354", "test", "devel")
	in := Install{Domains: []string{"test"}, Retire: []string{"devel"}, Addr: "127.0.0.1:5354"}
	steps := in.privilegedSteps()
	if len(steps) != 1 {
		t.Fatalf("retiring one domain produced %v", steps)
	}
	if err := in.applyAsRoot(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "devel")); !os.IsNotExist(err) {
		t.Error("the retired domain's entry is still there")
	}
	if _, err := os.Stat(filepath.Join(dir, "test")); err != nil {
		t.Error("retiring one domain removed another")
	}
}

func TestRetiringADomainWithNoEntryAsksForNothing(t *testing.T) {
	resolverFixture(t, "127.0.0.1:5354", "test")
	in := Install{Domains: []string{"test"}, Retire: []string{"devel"}, Addr: "127.0.0.1:5354"}
	if steps := in.privilegedSteps(); len(steps) != 0 {
		t.Errorf("retiring a domain that is not delegated asked for %v", steps)
	}
}

func TestANewDomainIsWritten(t *testing.T) {
	dir := resolverFixture(t, "127.0.0.1:5354", "test")
	in := Install{Domains: []string{"test", "devel"}, Addr: "127.0.0.1:5354"}
	if steps := in.privilegedSteps(); len(steps) != 1 {
		t.Fatalf("adding one domain produced %v", steps)
	}
	if err := in.applyAsRoot(); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "devel"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != in.resolverBody("devel") {
		t.Errorf("the entry written is %q", body)
	}
}

// An entry for a domain no state directory on this machine delegates is left in
// place and reported. A file under /etc that this tool cannot show it wrote is
// not this tool's to delete, and deleting one by its shape is what let one state
// directory withdraw another's delegations.
func TestUnclaimedEntriesAreReportedAndNotRemoved(t *testing.T) {
	dir := resolverFixture(t, "127.0.0.1:5354", "test", "devel")
	got := UnclaimedResolverEntries([]string{"test"}, "127.0.0.1:5354")
	if len(got) != 1 || got[0] != "devel" {
		t.Fatalf("UnclaimedResolverEntries = %v, want [devel]", got)
	}
	in := Install{Domains: []string{"test"}, Addr: "127.0.0.1:5354"}
	if err := in.applyAsRoot(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "devel")); err != nil {
		t.Error("an unclaimed entry was removed")
	}
}

// An entry another tool wrote is not reported: it points somewhere else, and
// naming it would send the reader after a file that is not theirs to explain.
func TestAnotherToolsEntryIsNotReported(t *testing.T) {
	dir := resolverFixture(t, "127.0.0.1:5354", "test")
	other := "domain other\nsearch other\nnameserver 127.0.0.1\nport 2053\n"
	if err := os.WriteFile(filepath.Join(dir, "other"), []byte(other), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := UnclaimedResolverEntries([]string{"test"}, "127.0.0.1:5354"); len(got) != 0 {
		t.Errorf("UnclaimedResolverEntries = %v, want none", got)
	}
}
