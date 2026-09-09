package stack

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Install describes the machine state this tool creates outside the user's home
// directory: the resolver entries that delegate the local domains, and the
// certificate authority in the user's trust settings.
type Install struct {
	Domains []string // local domains, e.g. ["test", "lab.internal"]
	Addr    string   // host:port bayd listens on
	CAPath  string   // CA certificate to trust; empty to skip
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
}

// Elevator runs one command with administrator rights.
type Elevator func(exe string, args []string) error

// Pending lists the steps that are not in place yet. An empty result means
// there is nothing to elevate for.
func (in Install) Pending() []string {
	steps := in.privilegedSteps()
	if in.UntrustCA != "" && CATrusted(in.UntrustCA) {
		steps = append(steps, "stop trusting the retired "+in.UntrustCA)
	}
	if in.CAPath != "" && !CATrusted(in.CAPath) {
		steps = append(steps, "trust "+in.CAPath+" in your keychain")
	}
	return steps
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
	for _, d := range in.Stale() {
		steps = append(steps, "remove "+ResolverPath(d))
	}
	return steps
}

// Stale returns domains this tool delegated earlier and no longer serves. Only
// entries whose contents point at this tool's listener are returned.
func (in Install) Stale() []string {
	entries, err := os.ReadDir(resolverDir)
	if err != nil {
		return nil
	}
	current := make(map[string]bool, len(in.Domains))
	for _, d := range in.Domains {
		current[strings.Trim(d, ".")] = true
	}
	var stale []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || current[name] {
			continue
		}
		b, err := os.ReadFile(filepath.Join(resolverDir, name))
		if err != nil || string(b) != in.resolverBody(name) {
			continue
		}
		stale = append(stale, name)
	}
	sort.Strings(stale)
	return stale
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
		if err := in.elevate(args...); err != nil {
			return err
		}
	}
	return in.applyTrust()
}

// Apply performs the steps that need root. It must run as root.
func (in Install) Apply() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("must run as root")
	}
	for _, d := range in.Domains {
		if in.resolverInstalled(d) {
			continue
		}
		if err := in.writeResolver(d); err != nil {
			return err
		}
	}
	for _, d := range in.Stale() {
		path := ResolverPath(d)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		fmt.Printf("removed %s\n", path)
	}
	return nil
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

// sudoRun runs the helper through sudo, which reads the password from the
// caller's terminal. An application has no terminal and supplies its own
// Elevator.
func sudoRun(exe string, args []string) error {
	if !hasTerminal() {
		return fmt.Errorf("administrator rights are needed and there is no terminal to ask on; " +
			"run \"bay install\" from a terminal")
	}
	cmd := exec.Command("sudo", append([]string{"--", exe}, args...)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
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
