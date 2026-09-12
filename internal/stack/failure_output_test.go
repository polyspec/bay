package stack

import (
	"strings"
	"testing"
)

// A service that stops took its reason with it into its log. A failure that
// names the stage without that reason leaves nothing to act on: a database that
// crashed was reported only as having stopped during its healthcheck, while the
// crash sat in the container's log.

func TestAFailureCarriesTheServicesOwnOutput(t *testing.T) {
	logs := "2026-09-12T04:31:40 [ERROR] InnoDB: Cannot allocate memory\n" +
		"stack_bottom = 0 thread_stack 0x100000\n" +
		"The manual page at http://dev.mysql.com/doc/mysql/en/crashing.html\n"
	err := failureWithLog("service mysql stopped during healthcheck", logs)
	text := err.Error()
	if !strings.Contains(text, "service mysql stopped during healthcheck") {
		t.Errorf("the failure lost its own sentence: %v", err)
	}
	if !strings.Contains(text, "Cannot allocate memory") {
		t.Errorf("the failure does not carry the service's output: %v", err)
	}
}

// A service that said nothing leaves the failure as it was, with no empty
// section pretending there is something to read.
func TestAFailureWithNoOutputIsLeftAsItIs(t *testing.T) {
	err := failureWithLog("service web stopped during healthcheck", "   \n\n")
	if err.Error() != "service web stopped during healthcheck" {
		t.Errorf("an empty log was added to the failure: %v", err)
	}
}

// The output is bounded. A service that printed for an hour must not push the
// failure's own sentence off the top of the screen.
func TestTheOutputCarriedIsBounded(t *testing.T) {
	var b strings.Builder
	for i := 0; i < FailureLogLines*3; i++ {
		b.WriteString("line\n")
	}
	b.WriteString("last line\n")
	err := failureWithLog("service web stopped during healthcheck", b.String())
	lines := strings.Count(err.Error(), "\n")
	if lines > FailureLogLines+4 {
		t.Errorf("the failure carried %d lines, more than the bound of %d", lines, FailureLogLines)
	}
	if !strings.Contains(err.Error(), "last line") {
		t.Error("the bound kept the beginning of the log rather than the end")
	}
}
