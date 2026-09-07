// Command baybar is the menu bar item and the control window for
// bay. The menu shows the machine state and opens the window; actions
// are performed in the window.
//
// Reads come from stack.Snapshot and actions go through stack.Runtime, which
// the command line also uses.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/systray"

	"github.com/polyspec/bay/internal/stack"
)

// maxGroupLines caps the project entries in the menu. Menu items can only be
// appended, so they are created at start-up and hidden when unused.
const maxGroupLines = 12

var (
	file     = flag.String("f", "", "Compose file or directory to register on start (optional)")
	dir      = flag.String("state", defaultState(), "bay state directory")
	addr     = flag.String("addr", stack.DefaultDNSAddr, "address bayd listens on")
	dnsBin   = flag.String("dnsbin", "", "path to the bayd binary (default: next to this one)")
	interval = flag.Duration("interval", 3*time.Second, "how often to re-read the machine state")
	show     = flag.Bool("show", false, "open the window on start instead of waiting for the menu")
)

type app struct {
	rt *stack.Runtime

	openItem    *systray.MenuItem
	machineItem *systray.MenuItem
	groupItems  []*systray.MenuItem

	// Menu titles are cached. Every menu call runs
	// performSelectorOnMainThread:waitUntilDone:YES, which does not execute
	// while a menu is open, so only changed values are written.
	machineTitle string
	groupTitles  []string
	icon         fill
	needsSetup   bool

	mu sync.Mutex
	// selected is the sidebar row shown in the detail pane.
	selected string
	busy     bool
	snap     stack.Snapshot
	message  string
	kind     string
}

func main() {
	flag.Parse()
	log.SetFlags(log.Ltime)

	m := stack.NewMachine(*dir)
	if *file != "" {
		if cfg, err := stack.LoadIn(m, *file); err == nil {
			m.Register(cfg.Ref())
		}
	}
	a := &app{rt: &stack.Runtime{
		Machine:   m,
		Addr:      *addr,
		DNSBin:    beside("bayd", *dnsBin),
		HelperBin: beside("bay", ""),
		Elevate:   elevate,
	}, icon: fill(-1)}
	actionHandler = a.handle
	promptHandler = a.handlePrompt
	systray.Run(a.onReady, func() {})
}

func (a *app) onReady() {
	systray.SetTemplateIcon(statusIcon(fillNone), statusIcon(fillNone))
	a.icon = fillNone
	systray.SetTooltip("bay")

	a.openItem = systray.AddMenuItem("Open bay", "")
	systray.AddSeparator()
	a.machineItem = systray.AddMenuItem("Reading the machine…", "")
	systray.AddSeparator()
	for i := 0; i < maxGroupLines; i++ {
		it := systray.AddMenuItem("", "")
		it.Hide()
		a.groupItems = append(a.groupItems, it)
		a.groupTitles = append(a.groupTitles, "")
	}
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit", "")

	go a.watch(quit)
	go a.loop()
	if *show {
		go func() {
			a.refresh()
			a.openWindow()
		}()
	}
}

func (a *app) watch(quit *systray.MenuItem) {
	go a.on(a.openItem, a.openWindow)
	go a.on(a.machineItem, a.machineClicked)
	for i := range a.groupItems {
		go a.on(a.groupItems[i], a.openWindow)
	}
	<-quit.ClickedCh
	systray.Quit()
}

// on runs fn for each click, one at a time.
func (a *app) on(item *systray.MenuItem, fn func()) {
	for range item.ClickedCh {
		fn()
	}
}

func (a *app) openWindow() {
	showWindow(a.currentPanel())
}

func (a *app) currentPanel() panel {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.selected == "" {
		a.selected = viewDashboard
	}
	return buildPanel(a.snap, a.busy, a.selected, a.message, a.kind)
}

// report writes the outcome of an action to the window. Notifications require
// a signed application and are not used.
func (a *app) report(kind, format string, args ...any) {
	a.mu.Lock()
	a.message, a.kind = fmt.Sprintf(format, args...), kind
	a.mu.Unlock()
	if windowVisible() {
		updateWindow(a.currentPanel())
	}
	if kind == "error" {
		log.Printf("%s", fmt.Sprintf(format, args...))
	}
}

// machineClicked applies the machine setup when it is incomplete, and otherwise
// opens the window.
func (a *app) machineClicked() {
	if !a.needsSetup {
		a.openWindow()
		return
	}
	a.handle("setup")
}

func (a *app) loop() {
	for {
		a.refresh()
		time.Sleep(*interval)
	}
}

func (a *app) refresh() {
	snap, err := stack.Take(a.rt.Machine, a.rt.Addr)
	if err != nil {
		a.setMachine("Cannot read the machine - " + err.Error())
		a.setIcon(fillNone)
		return
	}
	a.mu.Lock()
	a.snap = snap
	a.mu.Unlock()

	line, needsSetup := machineLine(snap)
	a.needsSetup = needsSetup
	a.setMachine(line)
	a.setIcon(iconFor(snap))
	a.setGroups(snap.Groups)
	if windowVisible() {
		updateWindow(a.currentPanel())
	}
}

// handle runs one action from the window or the menu. One action runs at a
// time, because actions start and stop containers and rewrite the proxy
// configuration.
func (a *app) handle(id string) {
	if strings.HasPrefix(id, "https://") || strings.HasPrefix(id, "http://") {
		exec.Command("open", id).Start()
		return
	}
	// Selecting a sidebar row changes the detail pane only.
	if rest, ok := strings.CutPrefix(id, "select:"); ok {
		a.mu.Lock()
		a.selected = rest
		a.mu.Unlock()
		if windowVisible() {
			updateWindow(a.currentPanel())
		}
		return
	}

	parts := strings.Split(id, ":")
	if parts[0] == "logs" && len(parts) == 3 {
		a.showServiceLogs(parts[1], parts[2])
		return
	}
	// Anything that asks a question first returns here through the dialog, so
	// these never fall through to the action runner.
	if a.ask(parts) {
		return
	}

	a.mu.Lock()
	if a.busy {
		a.mu.Unlock()
		a.report("error", "another action is still running")
		return
	}
	a.busy, a.message, a.kind = true, "", ""
	a.mu.Unlock()
	a.refreshWindowOnly()

	defer func() {
		a.mu.Lock()
		a.busy = false
		a.mu.Unlock()
		a.refresh()
	}()

	rt := *a.rt
	var last string
	rt.Progress = func(line string) { last = line }

	if err := a.run(&rt, parts); err != nil {
		if errors.Is(err, errCancelled) {
			a.report("info", "cancelled")
			return
		}
		a.report("error", "%s failed: %v", parts[0], err)
		return
	}
	if last != "" {
		a.report("info", "%s", last)
	}
}

// ask presents the dialog an action needs and reports whether it handled the
// click. The dialog returns an identifier with a "do-" prefix, so answering it
// runs the action.
func (a *app) ask(parts []string) bool {
	switch parts[0] {
	case "machine-domain-add":
		prompt("do-machine-domain-add", "Add a domain",
			"It is delegated for the whole machine, and every project can use it.",
			"lab.test", "")
	case "machine-domain-remove":
		confirm("do-machine-domain-remove:"+parts[1], "Stop delegating "+parts[1]+"?",
			"Its resolver entry is removed. Nothing under it will resolve.", "Remove", false)
	case "domain-rename":
		prompt("do-domain-rename:"+parts[1], "Domain for "+parts[1],
			"Services in this project default into it. Naming one here pins it in "+
				"the project's Compose file; the machine's default is used otherwise.",
			"test", parts[2])
	case "service-domain":
		prompt("do-service-domain:"+parts[1]+":"+parts[2], "Change the domain of "+parts[2],
			"It has to sit under one of the project's domains.", "app.test", parts[3])
	case "cert-remove":
		confirm("do-cert-remove:"+parts[1], "Remove the certificate for "+parts[1]+"?",
			"It is reissued automatically if a route still needs it.", "Remove", false)
	case "ca-rotate":
		confirm("do-ca-rotate", "Replace the certificate authority?",
			"Every certificate it signed is discarded and reissued, and the keychain "+
				"is updated, which asks for your password. Browsers already holding a "+
				"page open will need a reload.", "Replace", true)
	default:
		return false
	}
	return true
}

func (a *app) handlePrompt(id, value string) {
	a.handle(id + ":" + value)
}

func (a *app) run(rt *stack.Runtime, parts []string) error {
	if parts[0] == "setup" {
		return rt.EnsureInstalled()
	}
	if handled, err := a.runEdit(rt, parts); handled {
		return err
	}
	if len(parts) < 2 {
		return fmt.Errorf("unknown action %q", strings.Join(parts, ":"))
	}
	cfg, err := rt.LoadGroup(parts[1])
	if err != nil {
		return err
	}
	var names []string
	if len(parts) > 2 && parts[2] != "" {
		names = []string{parts[2]}
	}
	switch parts[0] {
	case "up":
		_, err = rt.Up(cfg)
	case "down":
		_, err = rt.Down(cfg)
	case "start":
		_, err = rt.StartServices(cfg, names)
	case "stop":
		_, err = rt.StopServices(cfg, names)
	case "restart":
		_, err = rt.RestartServices(cfg, names)
	default:
		return fmt.Errorf("unknown action %q", parts[0])
	}
	return err
}

// runEdit performs the actions that change configuration: domains in a
// project's Compose file, and certificates in the state directory. It reports
// whether it recognised the action.
func (a *app) runEdit(rt *stack.Runtime, parts []string) (bool, error) {
	m := rt.Machine
	ca, err := stack.LoadOrCreateCA(m.Dir)
	if err != nil {
		return true, err
	}

	switch parts[0] {
	case "do-machine-domain-add", "do-machine-domain-remove", "do-machine-domain-default":
		switch parts[0] {
		case "do-machine-domain-add":
			if err := m.AddDomain(parts[1]); err != nil {
				return true, err
			}
		case "do-machine-domain-remove":
			if err := m.RemoveDomain(parts[1]); err != nil {
				return true, err
			}
		case "do-machine-domain-default":
			if err := m.SetDefaultDomain(parts[1]); err != nil {
				return true, err
			}
		}
		if err := rt.EnsureInstalled(); err != nil {
			return true, err
		}
		_, err := stack.SyncProxy(m)
		return true, err

	case "do-domain-rename", "do-service-domain", "do-domain-remove":
		cfg, err := rt.LoadGroup(parts[1])
		if err != nil {
			return true, err
		}
		switch parts[0] {
		case "do-domain-rename":
			if err := stack.SetPrimaryDomain(cfg, parts[2]); err != nil {
				return true, err
			}
		case "do-domain-remove":
			if err := stack.RemoveDomain(cfg, parts[2]); err != nil {
				return true, err
			}
		case "do-service-domain":
			if err := stack.SetServiceDomain(cfg, parts[2], parts[3]); err != nil {
				return true, err
			}
		}
		// Re-read the file and register again, so the machine's domain set
		// matches the file before the resolver entries are applied.
		next, err := stack.LoadIn(m, cfg.Path())
		if err != nil {
			return true, err
		}
		if err := m.Register(next.Ref()); err != nil {
			return true, err
		}
		if err := rt.EnsureInstalled(); err != nil {
			return true, err
		}
		_, err = stack.SyncProxy(m)
		return true, err

	case "do-cert-remove":
		if err := ca.RemoveCertificate(m.CertDir(), parts[1]); err != nil {
			return true, err
		}
		_, err := stack.SyncProxy(m)
		return true, err

	case "cert-reissue":
		if err := ca.Reissue(m.CertDir(), parts[1]); err != nil {
			return true, err
		}
		_, err := stack.SyncProxy(m)
		return true, err

	case "cert-reissue-all":
		if err := os.RemoveAll(m.CertDir()); err != nil {
			return true, err
		}
		_, err := stack.SyncProxy(m)
		return true, err

	case "do-ca-rotate":
		retired, err := stack.RotateCA(m.Dir, m.CertDir())
		if err != nil {
			return true, err
		}
		rt.RetiredCA = retired
		if err := rt.EnsureInstalled(); err != nil {
			return true, fmt.Errorf("the new authority was created but not trusted: %w", err)
		}
		if retired != "" {
			os.Remove(retired)
		}
		_, err = stack.SyncProxy(m)
		return true, err
	}
	return false, nil
}

func (a *app) showServiceLogs(group, service string) {
	cfg, err := a.rt.LoadGroup(group)
	if err != nil {
		a.report("error", "logs: %v", err)
		return
	}
	targets, err := stack.SelectServices(cfg, []string{service})
	if err != nil {
		a.report("error", "logs: %v", err)
		return
	}
	var buf strings.Builder
	if err := stack.Logs(targets[0].ContainerName, false, 400, &buf); err != nil {
		fmt.Fprintf(&buf, "\n[%v]\n", err)
	}
	body := buf.String()
	if strings.TrimSpace(body) == "" {
		body = "(no output yet)"
	}
	showLogs(group+" / "+service, body)
}

func (a *app) refreshWindowOnly() {
	if windowVisible() {
		updateWindow(a.currentPanel())
	}
}

func (a *app) isBusy() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.busy
}

func (a *app) setMachine(line string) {
	if line == a.machineTitle {
		return
	}
	a.machineTitle = line
	a.machineItem.SetTitle(line)
}

func (a *app) setIcon(f fill) {
	if f == a.icon {
		return
	}
	a.icon = f
	systray.SetTemplateIcon(statusIcon(f), statusIcon(f))
}

func (a *app) setGroups(groups []stack.GroupStatus) {
	for i, item := range a.groupItems {
		var title string
		if i < len(groups) {
			title = groupLine(groups[i])
		}
		if i == maxGroupLines-1 && len(groups) > maxGroupLines {
			title = fmt.Sprintf("…and %d more", len(groups)-maxGroupLines+1)
		}
		if title == a.groupTitles[i] {
			continue
		}
		wasHidden := a.groupTitles[i] == ""
		a.groupTitles[i] = title
		if title == "" {
			item.Hide()
			continue
		}
		item.SetTitle(title)
		if wasHidden {
			item.Show()
		}
	}
}

// beside returns the path of a sibling executable in the application bundle, or
// on PATH. The application performs the privileged steps through the
// bay binary beside it.
func beside(name, override string) string {
	if override != "" {
		if abs, err := filepath.Abs(override); err == nil {
			return abs
		}
	}
	if exe, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(exe), name)
		if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
			return cand
		}
	}
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	return ""
}

func defaultState() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".bay"
	}
	return filepath.Join(home, ".bay")
}
