package stack

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// Install describes the machine state this tool creates outside the user's home
// directory: the resolver entries that delegate the local domains, and the
// certificate authority in the user's trust settings.
type Install struct {
	Domains []string // local domains, e.g. ["test", "lab.internal"]
	// Retire names domains to stop delegating. Their resolver entries are
	// removed. Withdrawing a delegation is asked for by name and never
	// inferred, so running a project cannot withdraw one it was not asked
	// about.
	Retire []string
	Addr   string // host:port bayd listens on
	CAPath string // CA certificate to trust; empty to skip
	// UntrustCA is a retired certificate to remove from the trust settings
	// before the new one is added.
	UntrustCA string
	// Helper is the executable run as root to apply the privileged steps. It
	// must accept -privileged-apply and -privileged-remove. Empty means this
	// executable.
	Helper string
	// Elevate runs the helper as root. Nil uses sudo, which requires a
	// terminal.
	Elevate Elevator
	// RelayBin is the bayd binary the relay daemon runs, and State the
	// state directory it reads. An empty RelayBin leaves the daemon out of the
	// setup.
	RelayBin string
	State    string
	// AppleDomain is the domain Apple `container` names containers in; empty
	// leaves it out of the setup.
	AppleDomain string
}

// Elevator runs one command with administrator rights.
type Elevator func(exe string, args []string) error

// Pending lists the steps that are not in place yet. An empty result means
// there is nothing to elevate for.
func (in Install) Pending() []SetupStep {
	var steps []SetupStep
	for _, item := range in.Items() {
		if !item.OK {
			steps = append(steps, item.SetupStep)
		}
	}
	return steps
}

// Items lists every part of the setup this Install covers, whether it is in
// place or not. The steps that need root come first, because they are applied
// first.
func (in Install) Items() []SetupItem {
	var items []SetupItem
	for _, d := range in.Domains {
		items = append(items, SetupItem{
			Name: ResolverPath(d), OK: in.resolverInstalled(d),
			SetupStep: SetupStep{Text: "write " + ResolverPath(d), Asks: AsksAdministrator},
		})
	}
	for _, d := range in.retiring() {
		items = append(items, SetupItem{
			Name:      ResolverPath(d),
			SetupStep: SetupStep{Text: "remove " + ResolverPath(d), Asks: AsksAdministrator},
		})
	}
	if in.RelayBin != "" {
		item := SetupItem{
			Name: "relay " + RelayDaemonLabel, OK: !in.relayPending(),
			SetupStep: SetupStep{Text: "register the peer relay " + RelayDaemonPath(), Asks: AsksAdministrator},
		}
		if in.relayStale() && RelayDaemonCurrent(in.RelayBin, in.State) {
			item.Name += " (runs an older " + in.RelayBin + ")"
			item.Text = "restart the peer relay " + RelayDaemonLabel
		}
		items = append(items, item)
	}
	items = append(items, appleDomainItems(in.AppleDomain)...)
	if in.UntrustCA != "" && CATrusted(in.UntrustCA) {
		items = append(items, SetupItem{
			Name:      "trust " + in.UntrustCA,
			SetupStep: SetupStep{Text: "stop trusting the retired " + in.UntrustCA, Asks: AsksTrustSettings},
		})
	}
	if in.CAPath != "" {
		items = append(items, SetupItem{
			Name: "trust " + in.CAPath, OK: CATrusted(in.CAPath),
			SetupStep: SetupStep{Text: "trust " + in.CAPath + " in your keychain", Asks: AsksTrustSettings},
		})
	}
	return items
}

// relayPending reports whether the relay daemon is missing, registered with
// other arguments, not running, or running a replaced executable.
func (in Install) relayPending() bool {
	return in.RelayBin != "" && !(RelayDaemonCurrent(in.RelayBin, in.State) && RelayDaemonLoaded() && !in.relayStale())
}

// relayStale reports whether the running daemon started before its executable
// was replaced.
func (in Install) relayStale() bool {
	return in.RelayBin != "" && residentStale(launchdPID("system/"+RelayDaemonLabel), in.RelayBin)
}

// SetupItem is one part of a machine's setup and whether it is in place. When it
// is not, its step says what "bay install" does about it; a step with
// no text is one install cannot take, and the name says what to do.
type SetupItem struct {
	Name string
	OK   bool
	// Agent marks a part that tells agents about the tool, which running a
	// project does not need.
	Agent bool
	SetupStep
}

// SetupStep is one part of the machine setup that is not in place yet, and what
// it asks a person for.
type SetupStep struct {
	Text string
	Asks Asks
}

// Asks says what a step puts in front of a person. A report that leaves a step
// unmarked sends a program with no one at the keyboard into a prompt it cannot
// answer, so every step states this.
type Asks string

const (
	// AsksNobody is a step that runs without anyone present.
	AsksNobody Asks = ""
	// AsksAdministrator is a step that needs root. Writing under /etc/resolver
	// is the only one.
	AsksAdministrator Asks = "administrator rights"
	// AsksTrustSettings is a step that changes the user's certificate trust
	// settings. It needs no root, and macOS still puts its own dialog on the
	// screen and waits for the login password.
	AsksTrustSettings Asks = "your login password, in a dialog macOS puts on the screen"
)

// PendingText returns the pending steps as lines, for a caller that shows them
// without their prompts.
func (in Install) PendingText() []string {
	steps := in.Pending()
	out := make([]string, 0, len(steps))
	for _, s := range steps {
		out = append(out, s.Text)
	}
	return out
}

// privilegedSteps returns the steps that require root. Writing under /etc
// requires root; changing the user's trust settings does not.
func (in Install) privilegedSteps() []string {
	var steps []string
	for _, d := range in.Domains {
		if !in.resolverInstalled(d) {
			steps = append(steps, "write "+ResolverPath(d))
		}
	}
	for _, d := range in.retiring() {
		steps = append(steps, "remove "+ResolverPath(d))
	}
	if in.relayStale() && RelayDaemonCurrent(in.RelayBin, in.State) {
		steps = append(steps, "restart the peer relay "+RelayDaemonLabel)
	} else if in.relayPending() {
		steps = append(steps, "register the peer relay "+RelayDaemonPath())
	}
	if appleDomainRootPending(in.AppleDomain) {
		steps = append(steps, "create the Apple container DNS domain "+in.AppleDomain)
	}
	return steps
}

// UnclaimedResolverEntries returns the domains /etc/resolver delegates to this
// tool's listener that the given domain list does not cover. They are reported
// and never removed: an entry could have been written by another state
// directory on this machine, and removing one because it has this tool's shape
// is how one state directory withdrew another's delegations.
func UnclaimedResolverEntries(domains []string, addr string) []string {
	entries, err := os.ReadDir(resolverDir)
	if err != nil {
		return nil
	}
	serving := make(map[string]bool, len(domains))
	for _, d := range domains {
		serving[strings.Trim(d, ".")] = true
	}
	in := Install{Addr: addr}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || serving[name] {
			continue
		}
		b, err := os.ReadFile(filepath.Join(resolverDir, name))
		if err != nil || string(b) != in.resolverBody(name) {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Ensure applies whatever is missing, acquiring root itself. It prints what it
// is about to do so the password prompt is never a surprise.
func (in Install) Ensure() error {
	if os.Geteuid() == 0 {
		return in.Apply()
	}
	// The two groups require different permissions and are applied
	// separately.
	if steps := in.privilegedSteps(); len(steps) > 0 {
		fmt.Fprintf(os.Stderr, "administrator rights are needed once to:\n")
		for _, s := range steps {
			fmt.Fprintf(os.Stderr, "  - %s\n", s)
		}
		args := []string{"-privileged-apply", "-domain", strings.Join(in.Domains, ","), "-addr", in.Addr}
		if len(in.Retire) > 0 {
			args = append(args, "-retire", strings.Join(in.Retire, ","))
		}
		if in.RelayBin != "" {
			args = append(args, "-relaybin", in.RelayBin, "-state", in.State)
		}
		if in.AppleDomain != "" {
			args = append(args, "-appledomain", in.AppleDomain)
		}
		if err := in.elevate(args...); err != nil {
			return err
		}
	}
	if err := setAppleDomain(in.AppleDomain); err != nil {
		return err
	}
	if err := restartAppleSystem(in.AppleDomain); err != nil {
		return err
	}
	return in.applyTrust()
}

// Apply performs the steps that need root. It must run as root.
func (in Install) Apply() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("must run as root")
	}
	return in.applyAsRoot()
}

// applyAsRoot writes the entries the delegated domains are missing and removes
// the entries of the domains being retired. It touches no other entry: a domain
// this command does not serve belongs to whoever delegated it.
func (in Install) applyAsRoot() error {
	for _, d := range in.Domains {
		if in.resolverInstalled(d) {
			continue
		}
		if err := in.writeResolver(d); err != nil {
			return err
		}
	}
	for _, d := range in.retiring() {
		path := ResolverPath(d)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		fmt.Printf("removed %s\n", path)
	}
	if in.relayPending() {
		if err := installRelayDaemon(in.RelayBin, in.State); err != nil {
			return err
		}
	}
	if appleDomainRootPending(in.AppleDomain) {
		if err := createAppleDomain(in.AppleDomain); err != nil {
			return err
		}
	}
	return nil
}

// retiring returns the named domains that still have an entry to remove.
func (in Install) retiring() []string {
	var out []string
	for _, d := range in.Retire {
		if _, err := os.Stat(ResolverPath(strings.Trim(d, "."))); err == nil {
			out = append(out, strings.Trim(d, "."))
		}
	}
	return out
}

// applyTrust updates the user's certificate trust settings. It does not require
// root.
func (in Install) applyTrust() error {
	if in.UntrustCA != "" && CATrusted(in.UntrustCA) {
		if err := untrustCA(in.UntrustCA); err != nil {
			return err
		}
	}
	if in.CAPath != "" && !CATrusted(in.CAPath) {
		return trustCA(in.CAPath)
	}
	return nil
}

// ElevateArgs runs this executable as root with these arguments.
func (in Install) ElevateArgs(args ...string) error { return in.elevate(args...) }

// elevate runs the helper as root through whichever mechanism the caller has.
func (in Install) elevate(args ...string) error {
	exe := in.Helper
	if exe == "" {
		var err error
		if exe, err = os.Executable(); err != nil {
			return err
		}
	}
	if _, err := os.Stat(exe); err != nil {
		return fmt.Errorf("cannot run the privileged helper: %w", err)
	}
	if in.Elevate != nil {
		return in.Elevate(exe, args)
	}
	return sudoRun(exe, args)
}

// resolverDir is the directory macOS reads per-domain resolver configuration
// from. Tests replace it with a temporary directory.
var resolverDir = "/etc/resolver"

func ResolverPath(domain string) string {
	return filepath.Join(resolverDir, strings.Trim(domain, "."))
}

func (in Install) resolverInstalled(domain string) bool {
	b, err := os.ReadFile(ResolverPath(domain))
	if err != nil {
		return false
	}
	return string(b) == in.resolverBody(domain)
}

func (in Install) resolverBody(domain string) string {
	host, port, err := net.SplitHostPort(in.Addr)
	if err != nil {
		return ""
	}
	dom := strings.Trim(domain, ".")
	return fmt.Sprintf("domain %s\nsearch %s\nnameserver %s\nport %s\n", dom, dom, host, port)
}

func (in Install) writeResolver(domain string) error {
	body := in.resolverBody(domain)
	if body == "" {
		return fmt.Errorf("invalid listen address %q", in.Addr)
	}
	if err := os.MkdirAll(resolverDir, 0o755); err != nil {
		return err
	}
	path := ResolverPath(domain)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s\n", path)
	return nil
}

// UninstallResolver removes the resolver entries, acquiring root if needed.
// helper is the executable to re-run as root; empty means this one.
func UninstallResolver(helper string, elevate Elevator, domains ...string) error {
	var present []string
	for _, d := range domains {
		if _, err := os.Stat(ResolverPath(d)); err == nil {
			present = append(present, d)
		}
	}
	if len(present) == 0 {
		return nil
	}
	if os.Geteuid() == 0 {
		for _, d := range present {
			if err := os.Remove(ResolverPath(d)); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	}
	return Install{Helper: helper, Elevate: elevate}.elevate(
		"-privileged-remove", "-domain", strings.Join(present, ","))
}

// CATrusted reports whether the certificate at path already verifies against
// the machine's trust settings.
func CATrusted(path string) bool {
	// verify-cert accepts a self-signed certificate that is in no keychain,
	// because such a certificate is its own root. The presence of the
	// certificate in a keychain is checked first.
	if !inKeychain(path) {
		return false
	}
	cmd := exec.Command("security", "verify-cert", "-c", path, "-p", "ssl")
	cmd.Stdout, cmd.Stderr = nil, nil
	return cmd.Run() == nil
}

// inKeychain reports whether this certificate is in one of the machine's
// keychains. The comparison uses the certificate bytes, not the subject name,
// because a replaced authority has the same subject name as its replacement.
func inKeychain(path string) bool {
	want, err := certificateBody(path)
	if err != nil || want == "" {
		return false
	}
	out, err := exec.Command("security", "find-certificate", "-a", "-p").Output()
	if err != nil {
		return false
	}
	for _, block := range strings.Split(string(out), "-----END CERTIFICATE-----") {
		if body, err := certificateBodyFrom(block); err == nil && body == want {
			return true
		}
	}
	return false
}

// certificateBody returns a certificate's base64 body with all whitespace
// removed, so that two PEM encodings of the same certificate compare equal.
func certificateBody(path string) (string, error) {
	pem, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return certificateBodyFrom(string(pem))
}

func certificateBodyFrom(pem string) (string, error) {
	_, rest, found := strings.Cut(pem, "-----BEGIN CERTIFICATE-----")
	if !found {
		return "", errNoCertificate
	}
	body, _, _ := strings.Cut(rest, "-----END CERTIFICATE-----")
	return strings.Join(strings.Fields(body), ""), nil
}

var errNoCertificate = errors.New("no certificate in PEM form")

// trustCA adds the authority to the user's trust settings. The -d flag would
// use the administrator store, which requires root and cannot present the
// confirmation the operation needs.
//
// The command names a keychain. Without one it exits zero and adds nothing.
func trustCA(path string) error {
	out, err := exec.Command("security", trustArgs(path)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("trusting %s: %s", path, strings.TrimSpace(string(out)))
	}
	// The command exits zero whether or not it changed anything, so the result
	// is read back. Adding a certificate already present changes nothing.
	if !CATrusted(path) {
		return fmt.Errorf("trusting %s: the command reported success and no keychain holds it", path)
	}
	fmt.Printf("trusted %s\n", path)
	return nil
}

// trustArgs returns the arguments used to add the authority, including the
// keychain it is added to.
func trustArgs(path string) []string {
	args := []string{"add-trusted-cert", "-r", "trustRoot"}
	if k := loginKeychain(); k != "" {
		args = append(args, "-k", k)
	}
	return append(args, path)
}

// loginKeychain returns the path of the user's login keychain, or an empty
// string when that path does not exist.
func loginKeychain() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	path := filepath.Join(home, "Library", "Keychains", "login.keychain-db")
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}

func untrustCA(path string) error {
	out, err := exec.Command("security", "remove-trusted-cert", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("untrusting %s: %s", path, strings.TrimSpace(string(out)))
	}
	// The command exits zero whether or not it changed anything, so the result
	// is read back.
	if CATrusted(path) {
		return fmt.Errorf("untrusting %s: the command reported success and a keychain still holds it", path)
	}
	fmt.Printf("stopped trusting %s\n", path)
	return nil
}

// sudoRun runs the helper as root. From a terminal it uses sudo, which reads
// the password there. Without one, on macOS, it asks in the system's
// administrator dialog, so a command run from a program or an editor without a
// terminal can still be answered by the person at the Mac. An application
// supplies its own Elevator.
func sudoRun(exe string, args []string) error {
	if hasTerminal() {
		cmd := exec.Command("sudo", append([]string{"--", exe}, args...)...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		return cmd.Run()
	}
	if runtime.GOOS != "darwin" {
		// Without a terminal, such as on a CI runner, sudo runs only where it
		// asks for no password; root itself never comes here.
		cmd := exec.Command("sudo", append([]string{"-n", "--", exe}, args...)...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("administrator rights are needed, there is no terminal to ask on, and sudo -n " +
				"asks for a password; run \"bay install\" as root or where sudo needs no password")
		}
		return nil
	}
	fmt.Fprintln(os.Stderr, "no terminal to ask on; macOS asks for an administrator password in a dialog")
	out, err := exec.Command("osascript", "-e", adminScript(exe, args)).CombinedOutput()
	os.Stdout.Write(adminOutput(out))
	if err != nil {
		if strings.Contains(string(out), "-128") {
			return fmt.Errorf("the administrator dialog was cancelled")
		}
		return fmt.Errorf("running as administrator: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// adminScript is the AppleScript that runs the helper with administrator
// privileges. Every argument is quoted for the shell, and the command for
// AppleScript.
func adminScript(exe string, args []string) string {
	words := []string{shellQuote(exe)}
	for _, a := range args {
		words = append(words, shellQuote(a))
	}
	return "do shell script " + appleScriptQuote(strings.Join(words, " ")+" 2>&1") +
		" with administrator privileges"
}

// adminOutput returns what the helper printed. AppleScript ends lines with
// carriage returns.
func adminOutput(out []byte) []byte {
	s := strings.ReplaceAll(string(out), "\r", "\n")
	if s != "" && !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return []byte(s)
}

func hasTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return false
	}
	tty.Close()
	return true
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func appleScriptQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
