package main

import (
	"strings"
	"testing"

	"github.com/polyspec/bay/internal/stack"
)

func closed() *bool { f := false; return &f }

// php in soksakim-hyper: its supervisor stopped php-fpm and port 9000 was
// closed. The reason names the port and says what was measured.
func TestAClosedPortIsNamedWithItsNumber(t *testing.T) {
	s := stack.ServiceStatus{Name: "php", State: "running", Internal: true, Port: 9000, Accepting: closed()}
	got := notWorking(s)
	if !strings.Contains(got, "9000") {
		t.Errorf("reason %q does not name the port", got)
	}
	if serviceDot(s) != "warn" {
		t.Errorf("dot %q, want warn", serviceDot(s))
	}
}

// The service's own words come from its state, which is the engine's.
func TestTheStateWordIsTheEnginesState(t *testing.T) {
	s := stack.ServiceStatus{Name: "php", State: "running", Internal: true, Port: 9000, Accepting: closed()}
	if got := serviceState(s); got != text.T("running") {
		t.Errorf("state %q, want the engine's state, running", got)
	}
}
