package stack

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// The machine setup is the resolver entries, the DNS agent and the proxy. Each
// of the three exists once per machine and cannot be divided: a resolver entry
// is keyed by domain and /etc/resolver is one directory per host, the agent is
// one launchd job carrying one list of domains and one address, and the proxy
// is one container name per engine.
//
// -state selects the directory holding the authority, the certificates and the
// project registry. It does not divide the machine setup, so one state
// directory owns it and the others read it.
//
// The owner is the state directory the registered agent runs with. Reading it
// from the program that answers the domains means the owner cannot disagree
// with what is actually serving them.

// MachineOwner returns the state directory that owns the machine setup and
// whether there is one. No registered agent, or one registered without a state
// directory, means no owner: the next install takes it.
func MachineOwner() (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	path := filepath.Join(home, "Library", "LaunchAgents", DNSAgentLabel+".plist")
	out, err := exec.Command("plutil", "-extract", "ProgramArguments", "json", "-o", "-", path).Output()
	if err != nil {
		return "", false
	}
	var args []string
	if err := json.Unmarshal(out, &args); err != nil {
		return "", false
	}
	return ownerFromArgs(args)
}

// ownerFromArgs reads the owner out of a registered job's arguments.
func ownerFromArgs(args []string) (string, bool) {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-state" && args[i+1] != "" {
			return args[i+1], true
		}
	}
	return "", false
}

// OwnsMachineSetup reports whether dir may change the machine setup. It is
// asked by every step that writes one of the three, and by nothing that reads.
func OwnsMachineSetup(dir string) error {
	owner, owned := MachineOwner()
	return ownershipHeldBy(dir, owner, owned)
}

// ownershipHeldBy compares a caller against the owner. Paths are resolved
// first: the same directory reached through a link or written with a trailing
// separator is one owner, and refusing it would refuse the owner itself.
func ownershipHeldBy(dir, owner string, owned bool) error {
	if !owned {
		return nil
	}
	if sameDir(dir, owner) {
		return nil
	}
	return fmt.Errorf(
		"this machine's setup belongs to the state directory %s, and this command "+
			"runs against %s. The resolver entries, the DNS agent and the proxy are "+
			"the machine's and there is one of each. Run the command with -state %s, "+
			"or take the setup over with: bay -state %s install",
		owner, dir, owner, dir)
}
