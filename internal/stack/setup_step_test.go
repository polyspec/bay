package stack

import "testing"

// Every step of the setup that is not in place says what it asks a person for.
// Two of them ask, and they ask for different things: writing under
// /etc/resolver needs administrator rights, and changing the trust settings
// makes macOS put its own dialog on the screen. Neither can be answered by a
// program with no one at the keyboard, so a report that leaves one of them
// unmarked sends a script into a prompt it cannot answer.
func TestEachPendingStepSaysWhatItAsksFor(t *testing.T) {
	useTempResolverDir(t)
	in := Install{Domains: []string{"test"}, Addr: "127.0.0.1:5354", CAPath: "/nowhere/ca.crt"}

	steps := in.Pending()
	if len(steps) != 2 {
		t.Fatalf("Pending() = %v, want the resolver write and the trust change", steps)
	}
	if steps[0].Asks != AsksAdministrator {
		t.Errorf("writing %s asks %q", ResolverPath("test"), steps[0].Asks)
	}
	if steps[1].Asks != AsksTrustSettings {
		t.Errorf("trusting the authority asks %q", steps[1].Asks)
	}
	for _, s := range steps {
		if s.Asks == AsksNobody {
			t.Errorf("step %q asks nobody, and both of these ask", s.Text)
		}
	}
}

// A machine with nothing outstanding asks for nothing, which is the ordinary
// case and the one a program runs in.
func TestASetupAlreadyInPlaceAsksForNothing(t *testing.T) {
	resolverFixture(t, "127.0.0.1:5354", "test")
	in := Install{Domains: []string{"test"}, Addr: "127.0.0.1:5354"}
	if steps := in.Pending(); len(steps) != 0 {
		t.Errorf("Pending() = %v, want none", steps)
	}
}
