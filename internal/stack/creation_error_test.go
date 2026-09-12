package stack

import (
	"strings"
	"testing"
)

// A creation failure names the thing the runtime refused over. A category on
// its own names nothing: a missing bind source reported as a rejected creation
// leaves the reader with no path to create.
func TestACreationFailureCarriesWhatTheRuntimeSaid(t *testing.T) {
	said := "Error: path '/Users/someone/orm/.runtime/db-tests' does not exist\n"
	args := []string{"create", "--name", "p-web", "docker.io/library/alpine:3"}
	got := creationFailure(said, args)
	if !strings.Contains(got, "/Users/someone/orm/.runtime/db-tests") {
		t.Errorf("the failure does not name the path: %q", got)
	}
	if !strings.Contains(got, "resource not found") {
		t.Errorf("the failure lost its category: %q", got)
	}
}

// Environment values ride on the command line, so a runtime that echoes an
// argument echoes a value with it. The pairs this command passed are redacted.
func TestEnvironmentValuesAreRedactedFromWhatTheRuntimeSaid(t *testing.T) {
	args := []string{"create", "--name", "p-db",
		"--env", "POSTGRES_PASSWORD=s3cr3t",
		"--env", "MYSQL_ALLOW_EMPTY_PASSWORD=yes",
		"docker.io/library/postgres:17"}
	said := "Error: invalid argument --env POSTGRES_PASSWORD=s3cr3t for container p-db\n"
	got := creationFailure(said, args)
	if strings.Contains(got, "s3cr3t") {
		t.Errorf("a password was printed: %q", got)
	}
	if !strings.Contains(got, "POSTGRES_PASSWORD=") {
		t.Errorf("the key was removed along with the value: %q", got)
	}
}

// Redacting a pair must not touch an ordinary word that reads like a short
// value. "yes" and "orm" are values here and also appear in the runtime's own
// words.
func TestRedactionDoesNotTouchOrdinaryWords(t *testing.T) {
	args := []string{"create", "--name", "orm-physical-db",
		"--env", "MYSQL_ALLOW_EMPTY_PASSWORD=yes",
		"--env", "POSTGRES_DB=orm_test",
		"docker.io/library/mysql:8.4"}
	said := "Error: container orm-physical-db: path '/Users/someone/orm/results' does not exist\n"
	got := creationFailure(said, args)
	if !strings.Contains(got, "/Users/someone/orm/results") {
		t.Errorf("redaction mangled the path: %q", got)
	}
	// "orm" and "yes" are environment values here and also read as ordinary
	// words in the runtime's sentence.
	if !strings.Contains(got, "orm-physical-db") {
		t.Errorf("redaction mangled the container name: %q", got)
	}
}

// A runtime that said nothing leaves the category to speak alone.
func TestACreationFailureWithNoDiagnosticKeepsItsCategory(t *testing.T) {
	got := creationFailure("   \n", []string{"create", "--name", "p-web"})
	if !strings.Contains(got, "runtime rejected creation") {
		t.Errorf("the category is missing: %q", got)
	}
	if strings.Contains(got, ":") && strings.HasSuffix(strings.TrimSpace(got), ":") {
		t.Errorf("an empty diagnostic left a dangling colon: %q", got)
	}
}
