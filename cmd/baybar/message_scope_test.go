package main

import "testing"

// An outcome belongs to the screen its action was started from. A Stop pressed
// on one project reported its failure on every screen opened afterwards, so
// another project's service appeared to have failed.
func TestAnOutcomeIsShownOnlyOnTheScreenItBelongsTo(t *testing.T) {
	crudui, other := viewProject+"crudui", viewService+"platform-sdk-local:registry"
	if m, k := messageFor(other, crudui, "down failed", "error"); m != "" || k != "" {
		t.Errorf("another screen shows %q (%s)", m, k)
	}
	if m, k := messageFor(crudui, crudui, "down failed", "error"); m != "down failed" || k != "error" {
		t.Errorf("its own screen shows %q (%s)", m, k)
	}
}
