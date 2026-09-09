package stack

import "testing"

// A machine reports what it still has to do, and trusting the authority is one
// of those steps. An authority the machine does not hold cannot be trusted by
// it, whatever a self-signed certificate says about itself: verifying one
// against itself succeeds without any keychain, so asking that question reports
// every fresh authority as trusted, leaves the trust step out of the pending
// list, and lets setup finish while every name the machine serves is refused by
// a browser with nothing said about it.
func TestAnAuthorityTheMachineDoesNotHoldIsNotTrusted(t *testing.T) {
	dir := t.TempDir()
	ca, err := LoadOrCreateCA(dir)
	if err != nil {
		t.Fatal(err)
	}
	if inKeychain(ca.CertPath()) {
		t.Fatal("an authority created a moment ago was found in a keychain")
	}
	if CATrusted(ca.CertPath()) {
		t.Fatal("an authority in no keychain is reported as trusted, so setup " +
			"reports nothing left to do while browsers refuse every name")
	}
}

// The answer does not change between two readings, so a machine that is asked
// twice is told the same thing and setup run twice does the same work.
func TestTheTrustAnswerIsTheSameEachTime(t *testing.T) {
	dir := t.TempDir()
	ca, err := LoadOrCreateCA(dir)
	if err != nil {
		t.Fatal(err)
	}
	first := CATrusted(ca.CertPath())
	for i := 0; i < 3; i++ {
		if CATrusted(ca.CertPath()) != first {
			t.Fatalf("reading %d disagreed with the first", i+2)
		}
	}
}

// A path with no certificate is not an authority the machine holds, and asking
// must answer rather than fail.
func TestAMissingAuthorityIsNotTrusted(t *testing.T) {
	if CATrusted(t.TempDir() + "/absent.crt") {
		t.Fatal("a certificate that is not there is reported as trusted")
	}
}

// The command that adds an authority exits zero whether or not it changed
// anything, and without a keychain named it changes nothing. Naming one is what
// makes it act, so the arguments carry it.
func TestTheTrustCommandNamesAKeychain(t *testing.T) {
	if loginKeychain() == "" {
		t.Skip("this machine has no login keychain to name")
	}
	args := trustArgs("/state/ca.crt")
	var named bool
	for i, a := range args {
		if a == "-k" && i+1 < len(args) && args[i+1] != "" {
			named = true
		}
	}
	if !named {
		t.Errorf("the trust command names no keychain, so it exits zero and adds nothing: %v", args)
	}
	if args[len(args)-1] != "/state/ca.crt" {
		t.Errorf("the authority is not the last argument: %v", args)
	}
}
