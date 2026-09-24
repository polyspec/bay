package main

import (
	"flag"
	"testing"
)

// The menu bar program registers a project only through "Add project…", the
// command meant for it. It takes no Compose file to register on start.
func TestStartTakesNoComposeFileToRegister(t *testing.T) {
	if f := flag.Lookup("f"); f != nil {
		t.Fatalf("flag -f exists: %s", f.Usage)
	}
}
