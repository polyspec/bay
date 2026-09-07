package main

import (
	"testing"
	"time"
)

// Several names share one resolver directory. A list of whole paths does not
// fit the row, and truncating it in the middle hides a name.
func TestResolverPaths(t *testing.T) {
	for _, c := range []struct {
		domains []string
		want    string
	}{
		{nil, "/etc/resolver/"},
		{[]string{"test"}, "/etc/resolver/test"},
		{[]string{"devel", "staging", "test"}, "/etc/resolver/{devel,staging,test}"},
	} {
		if got := resolverPaths(c.domains); got != c.want {
			t.Errorf("resolverPaths(%v) = %q, want %q", c.domains, got, c.want)
		}
	}
}

func TestHumanDuration(t *testing.T) {
	for _, c := range []struct {
		in   time.Duration
		want string
	}{
		{45 * time.Second, "45 s"},
		{90 * time.Second, "1 m 30 s"},
		{3*time.Hour + 12*time.Minute, "3 h 12 m"},
	} {
		if got := humanDuration(c.in); got != c.want {
			t.Errorf("humanDuration(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// A path outside the home directory is left as it is.
func TestShortPathLeavesForeignPaths(t *testing.T) {
	if got := shortPath("/tmp/guidecheck"); got != "/tmp/guidecheck" {
		t.Errorf("shortPath = %q", got)
	}
}
