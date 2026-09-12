package stack

import "testing"

// The agent is what runs on this machine between commands. One registered from
// a program that is no longer there stops at the next login, and one registered
// from another copy is not the installation being used, so neither is current.
func TestTheAgentIsNotCurrentWhenItRunsAnotherProgram(t *testing.T) {
	args := []string{"/opt/homebrew/bin/bayd", "-domain", "test",
		"-addr", "127.0.0.1:5354", "-state", "/Users/someone/.bay"}
	if !agentIsCurrent(args, []string{"test"}, "127.0.0.1:5354",
		"/opt/homebrew/bin/bayd", "/Users/someone/.bay") {
		t.Error("the agent of the installation being used is not current")
	}
	if agentIsCurrent(args, []string{"test"}, "127.0.0.1:5354",
		"/Applications/baybar.app/Contents/MacOS/bayd",
		"/Users/someone/.bay") {
		t.Error("an agent running another copy is current")
	}
}

// The settings still decide it: a machine that delegates another domain has an
// agent that is not current.
func TestTheAgentIsNotCurrentForOtherDomains(t *testing.T) {
	args := []string{"/opt/homebrew/bin/bayd", "-domain", "test",
		"-addr", "127.0.0.1:5354", "-state", "/Users/someone/.bay"}
	if agentIsCurrent(args, []string{"test", "devel"}, "127.0.0.1:5354",
		"/opt/homebrew/bin/bayd", "/Users/someone/.bay") {
		t.Error("an agent serving fewer domains is current")
	}
}

// Where the installation is has to be known to compare it. Without it the
// settings alone decide, which is what a reader that has no binary to name can
// still check.
func TestWithoutABinaryTheSettingsDecide(t *testing.T) {
	args := []string{"/anywhere/bayd", "-domain", "test",
		"-addr", "127.0.0.1:5354", "-state", "/Users/someone/.bay"}
	if !agentIsCurrent(args, []string{"test"}, "127.0.0.1:5354", "",
		"/Users/someone/.bay") {
		t.Error("the settings match but the agent is not current")
	}
}

// An agent registered for another state directory is not this installation's
// agent. Re-registering it is how the setup is taken over, and that is asked
// for before it happens.
func TestTheAgentIsNotCurrentForAnotherStateDirectory(t *testing.T) {
	args := []string{"/opt/homebrew/bin/bayd", "-domain", "test",
		"-addr", "127.0.0.1:5354", "-state", "/Users/someone/.bay"}
	if agentIsCurrent(args, []string{"test"}, "127.0.0.1:5354",
		"/opt/homebrew/bin/bayd", "/Users/someone/orm/.runtime/bay") {
		t.Error("an agent registered for another state directory is current")
	}
}
