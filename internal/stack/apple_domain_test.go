package stack

import "testing"

// The domain is written under [dns] and nothing else changes.
func TestTheAppleDomainIsWrittenUnderDNS(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"[build]\nrosetta = false\n\n[dns]\ndomain = \"container.test\"\n", "[build]\nrosetta = false\n\n[dns]\ndomain = \"bay.test\"\n"},
		{"[build]\nrosetta = false\n", "[build]\nrosetta = false\n\n[dns]\ndomain = \"bay.test\"\n"},
		{"", "[dns]\ndomain = \"bay.test\"\n"},
		{"[dns]\n# kept\n[x]\ndomain = \"y\"\n", "[dns]\ndomain = \"bay.test\"\n# kept\n[x]\ndomain = \"y\"\n"},
	} {
		if got := withAppleDomain(c.in, "bay.test"); got != c.want {
			t.Errorf("withAppleDomain(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
