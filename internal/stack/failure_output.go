package stack

import (
	"bytes"
	"fmt"
	"strings"
)

// FailureLogLines bounds how much of a failing service's log is carried with
// the failure. It is the tail, because the reason a service stopped is the last
// thing it said.
const FailureLogLines = 30

// failureWithLog attaches a service's own output to the failure that reports
// it. A service that stops took its reason with it into its log, and a failure
// that names the stage without that reason leaves nothing to act on.
//
// The output goes to the error the command prints and nowhere else. Nothing
// here writes it to the machine state or to a file, and it does not outlive the
// command.
func failureWithLog(reason, log string) error {
	tail := lastLines(log, FailureLogLines)
	if tail == "" {
		return fmt.Errorf("%s", reason)
	}
	return fmt.Errorf("%s\n\nwhat it said last:\n%s", reason, tail)
}

// lastLines returns the final n non-empty-trailing lines of s, indented so the
// service's words are not read as the tool's own.
func lastLines(s string, n int) string {
	s = strings.TrimRight(s, "\n \t\r")
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("  " + line)
	}
	return b.String()
}

// serviceLog returns what a container has said, for a failure to carry. A log
// that cannot be read returns empty: the failure is reported either way, and
// the reason it could not be read is not the failure being reported.
func (e *serviceEngine) serviceLog(name string) string {
	var out bytes.Buffer
	if err := Logs(name, false, FailureLogLines, &out); err != nil {
		return ""
	}
	return out.String()
}
