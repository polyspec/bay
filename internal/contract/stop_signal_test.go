package contract

import (
	"strings"
	"testing"
)

// The schema lists stop_signal with every name the contract accepts.
func TestStopSignalIsDeclaredWithItsNames(t *testing.T) {
	for _, k := range ComposeKeys() {
		if k.Path != "stop_signal" {
			continue
		}
		for _, name := range StopSignals() {
			if !strings.Contains(k.Description, name) {
				t.Fatalf("stop_signal description %q does not list %s", k.Description, name)
			}
		}
		return
	}
	t.Fatal("ComposeKeys has no stop_signal")
}
