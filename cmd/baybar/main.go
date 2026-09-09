// Command baybar is the menu bar item and the control window for
// bay. The menu shows the machine state and opens the window; actions
// are performed in the window.
//
// Reads come from stack.Snapshot and actions go through stack.Runtime, which
// the command line also uses.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/systray"

	"github.com/polyspec/bay/internal/i18n"
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
	// watcher keeps the machines announcing themselves on this network, so the
	// peers screen shows one as it appears.
	watcher *stack.Watcher

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
	applyLanguage()
	a := &app{rt: &stack.Runtime{
		Machine:   m,
		Addr:      *addr,
		DNSBin:    beside("bayd", *dnsBin),
		HelperBin: beside("bay", ""),
		Elevate:   elevate,
	}, icon: fill(-1)}
	a.watcher = stack.Watch(context.Background(), 0)
	actionHandler = a.handle
	promptHandler = a.handlePrompt
	sheetHandler = a.handleSheet
	systray.Run(a.onReady, func() {})
}

func (a *app) onReady() {
	systray.SetTemplateIcon(statusIcon(fillNone), statusIcon(fillNone))
	a.icon = fillNone
	systray.SetTooltip("bay")

	a.openItem = systray.AddMenuItem(text.T("Open bay"), "")
	systray.AddSeparator()
	a.machineItem = systray.AddMenuItem(text.T("Reading the machine…"), "")
	systray.AddSeparator()
	for i := 0; i < maxGroupLines; i++ {
		it := systray.AddMenuItem("", "")
		it.Hide()
		a.groupItems = append(a.groupItems, it)
		a.groupTitles = append(a.groupTitles, "")
	}
	systray.AddSeparator()
	quit := systray.AddMenuItem(text.T("Quit"), "")

	go a.watch(quit)
	go a.loop()
	if *show || showAtLaunch() {
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
	snap, busy, message, kind := a.snap, a.busy, a.message, a.kind
	if a.selected == "" {
		a.selected = viewDashboard
	}
	selected := a.selected
	a.mu.Unlock()

	// The service screen shows the tail of its container's output, so it is
	// read on the way to building the panel rather than held in the snapshot.
	var log []string
	if rest, ok := strings.CutPrefix(selected, viewService); ok {
		group, service, _ := strings.Cut(rest, ":")
		log = a.serviceLogTail(group, service, logTail)
	}
	return buildPanel(snap, busy, selected, message, kind, log, a.watcher.List())
}

// showAtLaunch reports whether the window opens with the application. The
// command line flag opens it once; the setting opens it every time.
func showAtLaunch() bool { return boolSetting("showAtLaunch") }

// languageChoice returns the stored language setting: "system", "en" or "ko".
func languageChoice() string { return stringSetting("language") }

// applyLanguage renders the window in the chosen language. The words the window
// builds its own controls from are sent again, because they change with it.
func applyLanguage() {
	setLanguage(languageChoice(), language())
	setLabels(windowLabels())
}

// setLanguageChoice stores the choice at the given position and redraws the
// window in that language.
func (a *app) setLanguageChoice(index string) {
	n, err := strconv.Atoi(index)
	if err != nil || n < 0 || n >= len(i18n.Choices) {
		return
	}
	setStringSetting("language", i18n.Choices[n])
	applyLanguage()
	a.refreshWindowOnly()
}

// report writes the outcome of an action to the window. Notifications require
// a signed application and are not used.
func (a *app) report(kind, format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	a.mu.Lock()
	a.message, a.kind = line, kind
	a.mu.Unlock()
	if windowVisible() {
		updateWindow(a.currentPanel())
	}
	// Every outcome is logged, not only the failures. What the window showed is
	// otherwise gone as soon as the next action replaces it.
	log.Printf("%s: %s", kind, line)
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
		a.setMachine(text.T("Cannot read the machine - %s", err.Error()))
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

	// Reveal takes a path, which contains colons on no supported layout but is
	// still read whole rather than split.
	if path, ok := strings.CutPrefix(id, "reveal:"); ok {
		exec.Command("open", "-R", path).Start()
		return
	}
	// The Compose file is read as it is on disk, not as it was parsed, because
	// it is the file the project is edited in.
	if path, ok := strings.CutPrefix(id, "view:"); ok {
		a.showFile(path)
		return
	}
	if value, ok := strings.CutPrefix(id, "copy:"); ok {
		copyText(value)
		a.report("info", "%s", text.T("copied %s", value))
		return
	}

	parts := strings.Split(id, ":")
	if parts[0] == "logs" && len(parts) == 3 {
		a.showServiceLogs(parts[1], parts[2])
		return
	}
	if parts[0] == "copy-log" && len(parts) == 3 {
		lines := a.serviceLogTail(parts[1], parts[2], 400)
		copyText(strings.Join(lines, "\n"))
		a.report("info", "%s", text.P("copied %d line", "copied %d lines",
			len(lines), len(lines)))
		return
	}
	if parts[0] == "doctor" {
		a.showDoctor()
		return
	}
	// The window re-reads the machine on a timer; this reads it now.
	if parts[0] == "refresh" {
		a.refresh()
		return
	}
	if parts[0] == "language" && len(parts) == 2 {
		a.setLanguageChoice(parts[1])
		return
	}
	if parts[0] == "toggle-show-at-launch" {
		setBoolSetting("showAtLaunch", !showAtLaunch())
		a.refreshWindowOnly()
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
		a.report("error", "%s", text.T("another action is still running"))
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
			a.report("info", "%s", text.T("cancelled"))
			return
		}
		a.report("error", "%s", text.T("%s failed: %v", parts[0], err))
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
	case "project-add":
		pick("do-project-add", text.T("Choose a project directory or its Compose file"),
			text.T("Add project"))
	case "machine-domain-add":
		// The sheet shows the resulting name while it is typed, because a
		// wildcard whose parent is a single label is rejected by clients.
		sheet("do-machine-domain-add", text.T("Add a domain"),
			text.T("bay will write /etc/resolver/<name> so every name under it "+
				"resolves to the local agent. macOS asks for your password once."),
			text.T("Domain"), "staging", ".",
			text.T("Make it the default for new projects"), text.T("Add domain"), false)
	case "machine-domain-default":
		prompt("do-machine-domain-default", text.T("Default domain"),
			text.T("Projects without a domain of their own use it. It has to be one of the "+
				"domains this machine already delegates."), "test", "")
	case "machine-uninstall":
		confirm("do-machine-uninstall", text.T("Remove the machine setup?"),
			text.T("The resolver entries and the DNS agent are removed, so names under the "+
				"delegated domains stop resolving. The authority stays trusted in your "+
				"keychain and the certificates stay in the state directory."),
			text.T("Remove"), true)
	case "machine-domain-remove":
		confirm("do-machine-domain-remove:"+parts[1], text.T("Stop delegating %s?", parts[1]),
			text.T("Its resolver entry is removed. Nothing under it will resolve."),
			text.T("Remove"), false)
	case "domain-rename":
		prompt("do-domain-rename:"+parts[1], text.T("Domain for %s", parts[1]),
			text.T("Services in this project default into it. Naming one here pins it in "+
				"the project's Compose file; the machine's default is used otherwise."),
			"test", parts[2])
	case "service-domain":
		prompt("do-service-domain:"+parts[1]+":"+parts[2],
			text.T("Change the domain of %s", parts[2]),
			text.T("It has to sit under one of the project's domains."), "app.test", parts[3])
	case "cert-remove":
		confirm("do-cert-remove:"+parts[1], text.T("Remove the certificate for %s?", parts[1]),
			text.T("It is reissued automatically if a route still needs it."),
			text.T("Remove"), false)
	case "peer-approve":
		// The address carries a colon, so what was split is put back together.
		address := strings.Join(parts[1:], ":")
		doc, err := stack.FetchPeer(address)
		if err != nil {
			a.report("error", "%s", err.Error())
			return true
		}
		fingerprint, err := stack.Fingerprint(doc.CA)
		if err != nil {
			a.report("error", "%s", err.Error())
			return true
		}
		confirm("do-peer-approve:"+address, text.T("Approve %s?", doc.Name),
			text.T("Its authority is %s. Compare it with what that machine reports. "+
				"Approving makes its domains reachable here and lets it reach this "+
				"machine's.", shortFingerprint(fingerprint)),
			text.T("Approve"), false)
	case "peer-remove":
		confirm("do-peer-remove:"+parts[1], text.T("Withdraw this approval?"),
			text.T("That machine's domains stop being reachable here, and it stops "+
				"reaching this machine's."), text.T("Remove"), false)
	case "cert-remove-unused":
		confirm("do-cert-remove-unused", text.T("Remove every unused certificate?"),
			text.T("They are the certificates no route and no registered project asks for. "+
				"A name a project asks for again is issued again."),
			text.T("Remove"), true)
	case "ca-rotate":
		confirm("do-ca-rotate", text.T("Replace the certificate authority?"),
			text.T("Every certificate it signed is discarded and reissued, and the keychain "+
				"is updated, which asks for your password. Browsers already holding a "+
				"page open will need a reload."), text.T("Replace"), true)
	default:
		return false
	}
	return true
}

func (a *app) handlePrompt(id, value string) {
	a.handle(id + ":" + value)
}

// handleSheet turns the sheet's answer into actions: the name first, then the
// choice made alongside it.
func (a *app) handleSheet(id, value string, option bool) {
	a.handle(id + ":" + value)
	if option && id == "do-machine-domain-add" {
		a.handle("do-machine-domain-default:" + value)
	}
}

func (a *app) run(rt *stack.Runtime, parts []string) error {
	if parts[0] == "setup" {
		return rt.EnsureInstalled()
	}
	if parts[0] == "peer-open" || parts[0] == "peer-close" {
		if err := rt.Machine.SetPeering(parts[0] == "peer-open"); err != nil {
			return err
		}
		if _, err := stack.SyncProxy(rt.Machine); err != nil {
			return err
		}
		if parts[0] == "peer-close" {
			rt.Progress(text.T("the link is closed"))
			return nil
		}
		rt.Progress(text.T("the link is open at %s", stack.PeerLinkAddress(stack.LANAddress())))
		return nil
	}
	if parts[0] == "do-peer-approve" && len(parts) > 1 {
		// The address is rejoined: it carries the port after a colon.
		address := strings.Join(parts[1:], ":")
		doc, err := stack.FetchPeer(address)
		if err != nil {
			return err
		}
		if err := stack.ApprovePeer(rt.Machine.Dir, stack.Peer{
			Name: doc.Name, Address: doc.Address, Domains: doc.Domains, CA: doc.CA,
		}); err != nil {
			return err
		}
		if _, err := stack.SyncProxy(rt.Machine); err != nil {
			return err
		}
		rt.Progress(text.T("approved %s", doc.Name))
		return nil
	}
	if parts[0] == "do-peer-remove" && len(parts) > 1 {
		if err := stack.RemovePeer(rt.Machine.Dir, parts[1]); err != nil {
			return err
		}
		if _, err := stack.SyncProxy(rt.Machine); err != nil {
			return err
		}
		rt.Progress(text.T("the approval is withdrawn"))
		return nil
	}
	if parts[0] == "proxy-sync" {
		// The proxy configuration is written by every action that starts or
		// stops a container, and by nothing else. This writes it from what is
		// running, so a change to how it is written reaches a machine whose
		// containers are already up.
		res, err := stack.SyncProxy(rt.Machine)
		if err != nil {
			return err
		}
		n := len(res.Routes)
		rt.Progress(text.P("rewrote the proxy configuration · %d route",
			"rewrote the proxy configuration · %d routes", n, n))
		return nil
	}
	if parts[0] == "proxy-restart" {
		// The proxy is generated from the containers that are running, so
		// removing it and syncing again re-publishes every route.
		for _, engine := range stack.Engines() {
			if err := stack.StopProxy(engine); err != nil {
				return err
			}
		}
		res, err := stack.SyncProxy(rt.Machine)
		if err != nil {
			return err
		}
		n := len(res.Routes)
		rt.Progress(text.P("rewrote the proxy configuration · %d route",
			"rewrote the proxy configuration · %d routes", n, n))
		return nil
	}
	if parts[0] == "do-machine-uninstall" {
		return a.uninstallMachine(rt)
	}
	if parts[0] == "do-project-add" {
		// The chosen path holds colons on no supported layout, but the action
		// was split on them, so it is joined back together.
		return a.addProject(rt, strings.Join(parts[1:], ":"))
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

	case "do-cert-remove-unused":
		// The list is read again here rather than carried through the dialog, so
		// a certificate that came into use while the question was open is kept.
		snap, err := stack.Take(m, rt.Addr)
		if err != nil {
			return true, err
		}
		names := unusedCertificates(snap)
		for _, name := range names {
			if err := ca.RemoveCertificate(m.CertDir(), name); err != nil {
				return true, err
			}
		}
		if _, err := stack.SyncProxy(m); err != nil {
			return true, err
		}
		rt.Progress(text.P("removed %d unused certificate", "removed %d unused certificates",
			len(names), len(names)))
		return true, nil

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

// addProject registers a Compose file so the window lists it. It is the same
// registration that "bay up" performs, without starting anything.
func (a *app) addProject(rt *stack.Runtime, path string) error {
	cfg, err := stack.LoadIn(rt.Machine, path)
	if err != nil {
		return err
	}
	if err := rt.Machine.Register(cfg.Ref()); err != nil {
		return err
	}
	a.mu.Lock()
	a.selected = viewProject + cfg.Name
	a.mu.Unlock()
	return nil
}

// uninstallMachine removes what the machine setup wrote: the resolver entries,
// which needs root, and the DNS agent, which does not.
func (a *app) uninstallMachine(rt *stack.Runtime) error {
	domains, err := rt.Machine.Domains()
	if err != nil {
		return err
	}
	if err := stack.UninstallDNSAgent(); err != nil {
		return err
	}
	return stack.UninstallResolver(rt.HelperBin, rt.Elevate, domains...)
}

// maxViewBytes caps what the text window is asked to hold. A Compose file is
// far smaller; a larger file is shown up to the cap and says so.
const maxViewBytes = 256 << 10

// showFile opens a file in the text window.
func (a *app) showFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		a.report("error", "%s", text.T("%s failed: %v", "view", err))
		return
	}
	defer f.Close()
	body, err := io.ReadAll(io.LimitReader(f, maxViewBytes+1))
	if err != nil {
		a.report("error", "%s", text.T("%s failed: %v", "view", err))
		return
	}
	if len(body) > maxViewBytes {
		body = append(body[:maxViewBytes],
			[]byte("\n\n"+text.T("shown up to %d KB", maxViewBytes>>10))...)
	}
	showLogs(shortPath(path), string(body), false)
}

// showDoctor runs the same report the command line prints and shows it in the
// text window.
func (a *app) showDoctor() {
	if a.rt.HelperBin == "" {
		a.report("error", "%s", text.T("doctor: bay is not beside this application"))
		return
	}
	out, err := exec.Command(a.rt.HelperBin, "-state", a.rt.Machine.Dir,
		"-addr", a.rt.Addr, "doctor").CombinedOutput()
	body := string(out)
	if err != nil {
		body += "\n[" + err.Error() + "]"
	}
	showLogs(text.T("bay doctor"), body, false)
}

// serviceLogTail returns the last lines of a service's output. An unreadable
// container reports why in place of the output.
func (a *app) serviceLogTail(group, service string, lines int) []string {
	cfg, err := a.rt.LoadGroup(group)
	if err != nil {
		return []string{err.Error()}
	}
	targets, err := stack.SelectServices(cfg, []string{service})
	if err != nil {
		return []string{err.Error()}
	}
	var buf strings.Builder
	if err := stack.Logs(targets[0].ContainerName, false, lines, &buf); err != nil {
		return []string{err.Error()}
	}
	out := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(out) == 1 && out[0] == "" {
		return nil
	}
	if len(out) > lines {
		out = out[len(out)-lines:]
	}
	return out
}

func (a *app) showServiceLogs(group, service string) {
	cfg, err := a.rt.LoadGroup(group)
	if err != nil {
		a.report("error", "%s", text.T("logs: %v", err))
		return
	}
	targets, err := stack.SelectServices(cfg, []string{service})
	if err != nil {
		a.report("error", "%s", text.T("logs: %v", err))
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
	showLogs(group+" / "+service, body, true)
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
			title = text.T("…and %d more", len(groups)-maxGroupLines+1)
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
