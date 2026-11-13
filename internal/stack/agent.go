package stack

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DNSAgentLabel identifies the launchd job that runs bayd. It is a user
// agent; bayd listens above port 1024 and does not require root.
const DNSAgentLabel = "dev.bay.dns"

// InstallDNSAgent writes the launchd job, loads it, and returns the plist
// path.
func InstallDNSAgent(exe, domain, addr, state, logDir string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, DNSAgentLabel+".plist")

	args := dnsAgentArgs(exe, domain, addr, state)
	plist := buildPlist(DNSAgentLabel, args,
		filepath.Join(logDir, "bayd.log"),
		filepath.Join(logDir, "bayd.err.log"))
	if err := os.WriteFile(path, []byte(plist), 0o644); err != nil {
		return "", err
	}

	target := "gui/" + strconv.Itoa(os.Getuid())
	// Boot out first so an edited plist is read. A missing job is not an error.
	exec.Command("launchctl", "bootout", target+"/"+DNSAgentLabel).Run()
	// Booting out returns before the job is gone, and bootstrapping one that is
	// still there fails. Wait for it to go.
	waitForAgentGone(target)
	if out, err := exec.Command("launchctl", "bootstrap", target, path).CombinedOutput(); err != nil {
		return "", fmt.Errorf("launchctl bootstrap: %s", strings.TrimSpace(string(out)))
	}
	return path, nil
}

// waitForAgentGone returns once launchd no longer knows the job, or after the
// deadline, which leaves bootstrapping to report what it finds.
func waitForAgentGone(target string) {
	deadline := time.Now().Add(5 * time.Second)
	for {
		if exec.Command("launchctl", "print", target+"/"+DNSAgentLabel).Run() != nil {
			return
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// UninstallDNSAgent stops the job and removes its plist.
func UninstallDNSAgent() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, "Library", "LaunchAgents", DNSAgentLabel+".plist")
	target := "gui/" + strconv.Itoa(os.Getuid())
	exec.Command("launchctl", "bootout", target+"/"+DNSAgentLabel).Run()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// dnsAgentArgs returns the job's argument list. The installer and the drift
// check both call it.
// dnsAgentArgs are the arguments the job is registered with. The state
// directory is among them because the registered job is where this machine's
// setup records its owner.
func dnsAgentArgs(exe, domain, addr, state string) []string {
	return []string{exe, "-domain", domain, "-addr", addr, "-state", state}
}

// DNSAgentServes reports whether the installed job runs with these settings. A
// renamed domain leaves the job registered with the previous values.
func DNSAgentServes(domains []string, addr, bin, state string) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	path := filepath.Join(home, "Library", "LaunchAgents", DNSAgentLabel+".plist")
	out, err := exec.Command("plutil", "-extract", "ProgramArguments", "json", "-o", "-", path).Output()
	if err != nil {
		return false
	}
	var got []string
	if err := json.Unmarshal(out, &got); err != nil || len(got) == 0 {
		return false
	}
	return agentIsCurrent(got, domains, addr, bin, state)
}

// agentIsCurrent reports whether the registered arguments are the ones this
// installation would register.
//
// The program is part of it. An agent registered from a copy that was moved or
// removed stops at the next login, and one registered from another copy is not
// the installation being used. bin is empty for a reader that has no
// installation to name, and then the settings alone decide.
func agentIsCurrent(got, domains []string, addr, bin, state string) bool {
	if len(got) == 0 {
		return false
	}
	if bin != "" && got[0] != bin {
		return false
	}
	want := dnsAgentArgs(got[0], strings.Join(domains, ","), addr, state)
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// DNSAgentLoaded reports whether launchd currently knows about the job.
func DNSAgentLoaded() bool {
	return exec.Command("launchctl", "print", dnsAgentTarget()).Run() == nil
}

// dnsAgentTarget is the job as launchctl names it.
func dnsAgentTarget() string {
	return "gui/" + strconv.Itoa(os.Getuid()) + "/" + DNSAgentLabel
}

func buildPlist(label string, args []string, stdout, stderr string) string {
	var b strings.Builder
	b.WriteString(xml.Header)
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString("<plist version=\"1.0\">\n<dict>\n")
	fmt.Fprintf(&b, "\t<key>Label</key>\n\t<string>%s</string>\n", esc(label))
	b.WriteString("\t<key>ProgramArguments</key>\n\t<array>\n")
	for _, a := range args {
		fmt.Fprintf(&b, "\t\t<string>%s</string>\n", esc(a))
	}
	b.WriteString("\t</array>\n")
	b.WriteString("\t<key>RunAtLoad</key>\n\t<true/>\n")
	b.WriteString("\t<key>KeepAlive</key>\n\t<true/>\n")
	fmt.Fprintf(&b, "\t<key>StandardOutPath</key>\n\t<string>%s</string>\n", esc(stdout))
	fmt.Fprintf(&b, "\t<key>StandardErrorPath</key>\n\t<string>%s</string>\n", esc(stderr))
	// launchd starts jobs with a search path that holds neither engine.
	b.WriteString("\t<key>EnvironmentVariables</key>\n\t<dict>\n" +
		"\t\t<key>PATH</key>\n\t\t<string>" + esc(agentSearchPath()) + "</string>\n\t</dict>\n")
	b.WriteString("</dict>\n</plist>\n")
	return b.String()
}

func esc(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s))
	return b.String()
}
