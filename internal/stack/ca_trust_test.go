package stack

import "testing"

// An authority that is in no keychain is not trusted. Verifying a self-signed
// certificate against itself succeeds without a keychain, so that check reports
// every new authority as trusted, omits the trust step from the pending list,
// and lets setup report success while a browser rejects every name the machine
// serves.
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

// The result does not change between calls, so setup run twice performs the
// same work.
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

// A path with no certificate returns false rather than an error.
func TestAMissingAuthorityIsNotTrusted(t *testing.T) {
	if CATrusted(t.TempDir() + "/absent.crt") {
		t.Fatal("a certificate that is not there is reported as trusted")
	}
}

// The command that adds an authority exits zero whether or not it changed
// anything, and without a named keychain it changes nothing.
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
