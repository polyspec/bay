package stack

import (
	"strings"
	"testing"
)

func TestDNSAgentArgs(t *testing.T) {
	got := dnsAgentArgs("/bin/bayd", "test,lab.internal", "127.0.0.1:5354", "edge")
	want := "/bin/bayd -domain test,lab.internal -addr 127.0.0.1:5354 -proxy edge"
	if strings.Join(got, " ") != want {
		t.Fatalf("dnsAgentArgs() = %q", strings.Join(got, " "))
	}
	got = dnsAgentArgs("/bin/bayd", "test", "127.0.0.1:5354", "")
	if strings.Contains(strings.Join(got, " "), "-proxy") {
		t.Fatalf("an empty proxy should not add a flag: %q", got)
	}
}
