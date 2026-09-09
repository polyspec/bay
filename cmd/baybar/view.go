package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/polyspec/bay/internal/i18n"
	"github.com/polyspec/bay/internal/stack"
)

// text renders every line the window shows. English is written here and Korean
// is looked up by it, so the two carry the same information.
var text = i18n.Printer{}

// setLanguage applies the stored choice against the locale the system prefers.
// It runs at start-up and again when the choice changes.
func setLanguage(choice, system string) {
	text = i18n.Printer{Lang: i18n.Resolve(choice, system)}
}

// languageLabels names the choices. A language is named in itself, so only the
// first is translated.
func languageLabels() []string {
	return []string{text.T("System"), "English", "한국어"}
}

// windowLabels are the words the window builds its own controls from.
func windowLabels() map[string]string {
	return map[string]string{
		"auto":     text.T("Auto"),
		"dark":     text.T("Dark"),
		"light":    text.T("Light"),
		"ok":       text.T("OK"),
		"cancel":   text.T("Cancel"),
		"then":     text.T("Then"),
		"noOutput": text.T("(no output yet)"),
	}
}

// The window is a source list: navigation on the left and one subject on the
// right. The first item is a dashboard, which shows the whole machine, because
// the source list displays only the selected item.

// Selections the sidebar can hold. A project is "project:<name>" and one of its
// services is "service:<project>:<name>".
const (
	viewDashboard = "dash"
	viewDomains   = "domains"
	viewCerts     = "certs"
	viewPeers     = "peers"
	viewSettings  = "settings"
	viewProject   = "project:"
	viewService   = "service:"
)

// logTail is how many lines the service screen shows under OUTPUT. The log
// window shows more.
const logTail = 12

// groupState is a project's run state. A running count is shown only for a
// partly running project; most projects on a machine are stopped, and a count
// on those would suggest a fault.
type groupState int

const (
	stateStopped groupState = iota
	statePartial
	stateRunning
)

// stateOf returns a project's state, how many of its services accept
// connections, and how many have a container running. The two differ while a
// container is up but not yet listening.
func stateOf(g stack.GroupStatus) (state groupState, running, live int) {
	for _, s := range g.Services {
		if s.Running() {
			running++
		}
		if s.Live() {
			live++
		}
	}
	switch {
	case live == 0:
		return stateStopped, running, live
	case running == len(g.Services):
		return stateRunning, running, live
	default:
		return statePartial, running, live
	}
}

func groupLine(g stack.GroupStatus) string {
	state, running, _ := stateOf(g)
	switch state {
	case stateStopped:
		return text.T("%s - stopped", g.Name)
	case statePartial:
		return text.T("%s - %d of %d running", g.Name, running, len(g.Services))
	default:
		return text.T("%s - running", g.Name)
	}
}

// machineLine returns the machine's status line and whether setup is
// incomplete. The menu bar shows it in one line.
func machineLine(snap stack.Snapshot) (string, bool) {
	if n := len(snap.Machine.Pending); n > 0 {
		return text.P("Finish setup (%d step, asks for your password)",
			"Finish setup (%d steps, asks for your password)", n, n), true
	}
	if proxyDown(snap) {
		return text.T("Nothing is reachable - the proxy is not answering"), false
	}
	if !snap.Machine.ProxiesServing() {
		return text.T("Nothing is running"), false
	}
	n := snap.Machine.ServingRoutes()
	return text.P("Serving %d domain", "Serving %d domains", n, n), false
}

// proxyDown reports that containers are up but the proxy is not serving their
// routes, so every address answers with nothing.
func proxyDown(snap stack.Snapshot) bool {
	if liveServices(snap) == 0 {
		return false
	}
	if snap.Machine.ServingRoutes() == 0 {
		return false
	}
	return !snap.Machine.ProxiesServing()
}

func liveServices(snap stack.Snapshot) int {
	n := 0
	for _, g := range snap.Groups {
		for _, s := range g.Services {
			if s.Live() {
				n++
			}
		}
	}
	return n
}

// iconFor returns the menu bar icon state. A stopped project is not reported as
// a fault.
func iconFor(snap stack.Snapshot) fill {
	if proxyDown(snap) {
		return fillNone
	}
	if !snap.Machine.ProxiesServing() {
		return fillNone
	}
	if len(snap.Machine.Pending) > 0 {
		return fillHalf
	}
	for _, g := range snap.Groups {
		// A partly running project differs from its Compose file.
		if state, _, _ := stateOf(g); state == statePartial {
			return fillHalf
		}
	}
	return fillFull
}

// buildPanel turns the snapshot into the window. busy disables every control,
// because an action already changing containers should not be raced. log holds
// the tail of the selected service's output, and is empty on every other
// screen.
func buildPanel(snap stack.Snapshot, busy bool, selected, message, kind string, log []string,
	found []stack.Beacon) panel {
	p := panel{
		Sidebar:     sidebar(snap, selected),
		Message:     message,
		MessageKind: kind,
	}
	switch {
	case selected == viewDomains:
		domainsView(&p, snap, busy)
	case selected == viewCerts:
		certificatesView(&p, snap, busy)
	case selected == viewPeers:
		peersView(&p, snap, busy, found)
	case selected == viewSettings:
		settingsView(&p, snap, busy)
	case strings.HasPrefix(selected, viewService):
		group, name, _ := strings.Cut(strings.TrimPrefix(selected, viewService), ":")
		if g, ok := findGroup(snap, group); ok {
			if s, ok := findService(g, name); ok {
				serviceView(&p, snap, g, s, busy, log)
				return p
			}
		}
		dashboardView(&p, snap, busy)
	case strings.HasPrefix(selected, viewProject):
		name := strings.TrimPrefix(selected, viewProject)
		if g, ok := findGroup(snap, name); ok {
			projectView(&p, g, busy)
			return p
		}
		fallthrough
	default:
		dashboardView(&p, snap, busy)
	}
	return p
}

func findGroup(snap stack.Snapshot, name string) (stack.GroupStatus, bool) {
	for _, g := range snap.Groups {
		if g.Name == name {
			return g, true
		}
	}
	return stack.GroupStatus{}, false
}

func findService(g stack.GroupStatus, name string) (stack.ServiceStatus, bool) {
	for _, s := range g.Services {
		if s.Name == name {
			return s, true
		}
	}
	return stack.ServiceStatus{}, false
}

// expandedProject returns the project whose services the sidebar lists, and the
// selected service inside it. Only the project being looked at is expanded.
func expandedProject(selected string) (group, service string) {
	if rest, ok := strings.CutPrefix(selected, viewService); ok {
		group, service, _ = strings.Cut(rest, ":")
		return group, service
	}
	if name, ok := strings.CutPrefix(selected, viewProject); ok {
		return name, ""
	}
	return "", ""
}

func sidebar(snap stack.Snapshot, selected string) []sideGroup {
	machine := sideGroup{Title: text.T("MACHINE"), Items: []sideItem{
		{ID: "select:" + viewDashboard, Label: text.T("Dashboard"),
			Dot: dashboardDot(snap), Selected: selected == viewDashboard},
		{ID: "select:" + viewDomains, Label: text.T("Domains"),
			Dot: "on", Count: fmt.Sprint(len(snap.Machine.DomainList)),
			Selected: selected == viewDomains},
		{ID: "select:" + viewCerts, Label: text.T("Certificates"),
			Dot:   dotFor(snap.Machine.CATrusted),
			Count: fmt.Sprint(len(snap.Certificates.Issued)), Selected: selected == viewCerts},
		{ID: "select:" + viewPeers, Label: text.T("Peers"),
			Dot:   peersDot(snap),
			Count: fmt.Sprint(len(snap.Machine.Peers)), Selected: selected == viewPeers},
		{ID: "select:" + viewSettings, Label: text.T("Settings"),
			Dot: "on", Selected: selected == viewSettings},
	}}

	expanded, selectedService := expandedProject(selected)
	down := proxyDown(snap)
	projects := sideGroup{Title: text.T("PROJECTS")}
	for _, g := range snap.Groups {
		state, running, _ := stateOf(g)
		dot := sideDot(state)
		if down && dot == "on" {
			dot = "bad"
		}
		item := sideItem{
			ID:       "select:" + viewProject + g.Name,
			Label:    g.Name,
			Dot:      dot,
			Count:    fmt.Sprintf("%d/%d", running, len(g.Services)),
			Selected: selected == viewProject+g.Name,
		}
		projects.Items = append(projects.Items, item)
		if g.Name != expanded {
			continue
		}
		for _, s := range g.Services {
			projects.Items = append(projects.Items, sideItem{
				ID:       "select:" + viewService + g.Name + ":" + s.Name,
				Label:    s.Name,
				Dot:      serviceDot(s),
				Sub:      true,
				Selected: s.Name == selectedService,
			})
		}
	}
	if len(projects.Items) == 0 {
		return []sideGroup{machine}
	}
	return []sideGroup{machine, projects}
}

// peersDot is off while the link is closed: a machine that is not answering is
// not a fault, it is a machine that was not asked to answer.
func peersDot(snap stack.Snapshot) string {
	if !snap.Machine.Peering {
		return "off"
	}
	return "on"
}

// peersView shows the link, the machines approved, and the machines announcing
// themselves that are not approved yet.
func peersView(p *panel, snap stack.Snapshot, busy bool, found []stack.Beacon) {
	p.Header = header{
		Title:    text.T("Peers"),
		Subtitle: text.T("machines whose domains this one reaches"),
	}
	if snap.Machine.Peering {
		p.Header.Buttons = []button{quiet("peer-close", text.T("Close the link"), busy)}
	} else {
		p.Header.Buttons = []button{hero("peer-open", text.T("Open the link"), busy)}
	}

	link := section{Header: text.T("LINK")}
	if snap.Machine.Peering {
		link.Rows = []row{
			{Text: text.T("Answering at"), Kind: "kv", Detail: snap.Machine.Link, Mono: true,
				Faint: text.T("give this to the other machine")},
			{Text: text.T("Announcing"), Kind: "kv", Detail: text.T("every second and a half"),
				Faint: text.T("a machine on this network finds it without the address")},
		}
	} else {
		link.Note = text.T("The link is closed. No machine reaches this one, and this one " +
			"announces nothing.")
	}
	p.Sections = append(p.Sections, link)

	approved := section{Header: text.T("APPROVED")}
	for _, peer := range snap.Machine.Peers {
		approved.Rows = append(approved.Rows, row{
			Text: peer.Name, Wide: true, Dot: "on",
			LinkText: peer.Address,
			Detail:   shortFingerprint(peer.Fingerprint),
			Buttons: []button{
				quiet("peer-remove:"+peer.Fingerprint, text.T("Remove"), busy),
			},
		})
	}
	if len(approved.Rows) == 0 {
		approved.Note = text.T("No machine is approved. Its domains are reachable here once it is.")
	}
	p.Sections = append(p.Sections, approved)

	// A machine already approved is not offered again, and neither is this one.
	known := map[string]bool{}
	for _, peer := range snap.Machine.Peers {
		known[peer.Fingerprint] = true
	}
	heard := section{Header: text.T("ON THIS NETWORK"),
		Note: text.T("announcing themselves right now")}
	for _, b := range found {
		if known[b.ID] || b.Address == snap.Machine.Link {
			continue
		}
		heard.Rows = append(heard.Rows, row{
			Text: b.Name, Wide: true, Dot: "off",
			LinkText: b.Address,
			Detail:   shortFingerprint(b.ID),
			Buttons:  []button{quiet("peer-approve:"+b.Address, text.T("Approve…"), busy)},
		})
	}
	if len(heard.Rows) == 0 {
		heard.Note = text.T("Nothing is announcing itself. A machine on another network is " +
			"reached by its address.")
	}
	p.Sections = append(p.Sections, heard)
}

// shortFingerprint returns what a person compares between two machines.
func shortFingerprint(fingerprint string) string {
	if len(fingerprint) > 16 {
		return fingerprint[:16]
	}
	return fingerprint
}

func dashboardDot(snap stack.Snapshot) string {
	switch {
	case proxyDown(snap):
		return "bad"
	case len(snap.Machine.Pending) > 0:
		return "warn"
	default:
		return "on"
	}
}

func dashboardView(p *panel, snap stack.Snapshot, busy bool) {
	needsSetup := len(snap.Machine.Pending) > 0
	down := proxyDown(snap)
	// The sidebar has no room for it, so registering a project sits here.
	p.Header = header{Title: text.T("Dashboard"),
		Buttons: []button{quiet("project-add", text.T("Add project…"), busy)}}

	running, total := 0, 0
	for _, g := range snap.Groups {
		if state, _, _ := stateOf(g); state != stateStopped {
			running++
		}
		total += len(g.Services)
	}
	live := liveServices(snap)
	ip := strings.Join(snap.Machine.ProxyAddrs(), ", ")

	switch {
	case down:
		sub := text.P("%d service running · the proxy is not answering",
			"%d services running · the proxy is not answering", live, live)
		if ip != "" {
			sub = text.T("%s on %s", sub, ip)
		}
		p.Verdict = &verdict{Dot: "bad", Headline: text.T("Nothing is reachable"), Subline: sub}
		routes := snap.Machine.ServingRoutes()
		p.Banner = &banner{
			Kind:  "bad",
			Title: text.T("The proxy is not running"),
			Text: text.P("The containers are up, so no work is lost. The proxy is created by "+
				"any command that produces routes; restarting it re-publishes all %d route.",
				"The containers are up, so no work is lost. The proxy is created by "+
					"any command that produces routes; restarting it re-publishes all %d routes.",
				routes, routes),
			Buttons: []button{
				quiet("doctor", text.T("Run doctor"), busy),
				hero("proxy-restart", text.T("Restart the proxy"), busy),
			},
		}
	case live == 0:
		p.Verdict = &verdict{
			Dot:      dotFor(!needsSetup),
			Headline: text.T("Not serving yet"),
			Subline: text.P("%d project registered · nothing running",
				"%d projects registered · nothing running", len(snap.Groups), len(snap.Groups)),
		}
	default:
		parts := []string{
			text.P("%d of %d project running", "%d of %d projects running",
				len(snap.Groups), running, len(snap.Groups)),
			text.P("%d service", "%d services", total, total),
		}
		if ip != "" {
			parts = append(parts, text.T("proxy %s", ip))
		}
		routes := snap.Machine.ServingRoutes()
		p.Verdict = &verdict{
			Dot:      dotFor(!needsSetup),
			Headline: text.P("Serving %d domain", "Serving %d domains", routes, routes),
			Subline:  strings.Join(parts, " · "),
		}
	}

	if needsSetup && !down {
		p.Verdict.Dot = "warn"
		p.Banner = &banner{
			Title:   text.T("This machine cannot serve the domains yet"),
			Text:    text.T("%s. It asks for your password once.", strings.Join(snap.Machine.Pending, ", ")),
			Buttons: []button{hero("setup", text.T("Finish setup"), busy)},
		}
	}

	projects := section{Header: text.T("PROJECTS")}
	for _, g := range snap.Groups {
		state, running, live := stateOf(g)
		detail := text.T("stopped")
		// Nothing can start before the resolver entries and the authority are
		// in place, so the action is offered but not enabled.
		action := hero("up:"+g.Name, text.T("Start"), busy || needsSetup)
		switch state {
		case stateRunning:
			detail = text.T("%d of %d running", running, len(g.Services))
			action = quiet("down:"+g.Name, text.T("Stop"), busy)
		case statePartial:
			detail = text.T("%d of %d running", running, len(g.Services))
			if live < len(g.Services) {
				action = hero("up:"+g.Name, text.T("Start the rest"), busy)
			} else {
				action = quiet("down:"+g.Name, text.T("Stop"), busy)
			}
		}
		dots := make([]string, 0, len(g.Services))
		for _, s := range g.Services {
			dots = append(dots, serviceDot(s))
		}
		projects.Rows = append(projects.Rows, row{
			Text: g.Name, Dot: sideDot(state), Chip: g.Domain, Dots: dots,
			Detail: detail, Buttons: []button{action},
			ID: "select:" + viewProject + g.Name,
		})
	}
	if len(projects.Rows) == 0 {
		projects.Note = text.T("No projects yet. Run \"bay up\" in a project directory.")
	}
	p.Sections = append(p.Sections, projects)

	addrs := section{Header: text.T("ADDRESSES"),
		Note: text.T("everything the proxy is serving right now")}
	if down {
		addrs.Note = text.T("every route the proxy would serve")
	}
	for _, g := range snap.Groups {
		for _, s := range g.Services {
			if s.Internal || !s.Live() {
				continue
			}
			r := row{Text: s.Name, Dot: "on", Link: s.URL, LinkText: s.URL, Detail: g.Name}
			switch {
			case down:
				r.Dot, r.Link, r.Detail = "bad", "", text.T("no answer")
			case !s.Routed:
				r.Dot, r.Link, r.Detail = "warn", "", text.T("502 · not listening")
			}
			addrs.Rows = append(addrs.Rows, r)
		}
	}
	if len(addrs.Rows) > 0 {
		p.Sections = append(p.Sections, addrs)
	}
}

func projectView(p *panel, g stack.GroupStatus, busy bool) {
	state, running, live := stateOf(g)
	p.Header = header{Title: g.Name, Subtitle: g.Domain + " · " + shortPath(filepath.Dir(g.StackPath))}
	if live < len(g.Services) {
		p.Header.Buttons = append(p.Header.Buttons,
			hero("up:"+g.Name, startTitle(len(g.Services)-live, live == 0), busy))
	}
	if live > 0 {
		p.Header.Buttons = append(p.Header.Buttons,
			quiet("restart:"+g.Name, text.T("Restart all"), busy),
			button{ID: "down:" + g.Name, Title: text.T("Stop"), Disabled: busy})
	}

	p.Verdict = projectVerdict(g, state, running)

	services := section{Header: text.T("SERVICES")}
	if g.Error != "" {
		services.Note = g.Error
	}
	for _, s := range g.Services {
		link, linkText := s.URL, s.URL
		detail := s.IPv4
		if detail == "" {
			detail = s.State
		}
		if s.Internal {
			link, linkText = "", text.T("internal · no route")
		} else if s.Live() && !s.Routed {
			detail = s.State
		}
		services.Rows = append(services.Rows, row{
			Text: s.Name, Dot: serviceDot(s), Link: link, LinkText: linkText, Detail: detail,
			ID:      "select:" + viewService + g.Name + ":" + s.Name,
			Buttons: []button{quiet("logs:"+g.Name+":"+s.Name, text.T("Logs"), s.State == "absent")},
		})
	}
	p.Sections = append(p.Sections, services)

	pinned := text.T("inherited from the machine")
	if g.DomainPinned {
		pinned = text.T("pinned in this project's Compose file")
	}
	p.Sections = append(p.Sections, section{
		Header: text.T("PROJECT"),
		Rows: []row{
			{Text: text.T("Domain"), Kind: "kv", Detail: g.Domain, Faint: pinned,
				Buttons: []button{quiet("domain-rename:"+g.Name+":"+g.Domain,
					text.T("Use another…"), busy)}},
			{Text: text.T("Compose file"), Kind: "kv", Detail: filepath.Base(g.StackPath),
				Faint: shortPath(filepath.Dir(g.StackPath)),
				Buttons: []button{
					quiet("view:"+g.StackPath, text.T("View"), false),
					quiet("reveal:"+g.StackPath, text.T("Reveal"), false),
				}},
			{Text: text.T("Routes"), Kind: "kv",
				Detail: text.P("%d of %d service", "%d of %d services",
					len(g.Services), routedCount(g), len(g.Services))},
		},
	})
}

// projectVerdict states the run count and, when it is short of the Compose
// file, why.
func projectVerdict(g stack.GroupStatus, state groupState, running int) *verdict {
	total := len(g.Services)
	if state == stateStopped {
		return &verdict{Dot: "", Headline: text.T("Nothing running"),
			Subline: text.P("%d service in the Compose file",
				"%d services in the Compose file", total, total)}
	}
	if state == stateRunning {
		return &verdict{Dot: "on",
			Headline: text.P("%d of %d service running", "%d of %d services running",
				total, running, total)}
	}
	var starting, stopped []string
	for _, s := range g.Services {
		switch {
		case s.State == "starting":
			starting = append(starting, s.Name)
		case !s.Live():
			stopped = append(stopped, s.Name)
		}
	}
	var why []string
	if len(starting) > 0 {
		why = append(why, text.T("%s: running but not listening yet, so not counted",
			strings.Join(starting, ", ")))
	}
	if len(stopped) > 0 {
		why = append(why, text.T("%s: not started", strings.Join(stopped, ", ")))
	}
	return &verdict{Dot: "warn",
		Headline: text.P("%d of %d service running", "%d of %d services running",
			total, running, total),
		Subline: strings.Join(why, " · ")}
}

func serviceView(p *panel, snap stack.Snapshot, g stack.GroupStatus, s stack.ServiceStatus, busy bool, log []string) {
	p.Header = header{
		Title:    s.Name,
		Subtitle: g.Name + " · " + serviceState(s),
		Buttons: []button{
			quiet("restart:"+g.Name+":"+s.Name, text.T("Restart"), busy || !s.Live()),
			quiet("stop:"+g.Name+":"+s.Name, text.T("Stop"), busy || !s.Live()),
		},
	}
	if !s.Live() {
		p.Header.Buttons = append([]button{
			hero("start:"+g.Name+":"+s.Name, text.T("Start"), busy)}, p.Header.Buttons...)
	}

	route := section{Header: text.T("ROUTE")}
	if s.Internal {
		route.Rows = []row{{Text: text.T("Address"), Kind: "kv",
			Detail: text.T("internal · no route"),
			Faint:  text.T("the service names no domain, so the proxy does not forward to it")}}
	} else {
		reach := text.T("the proxy forwards it to this container")
		if !s.Routed {
			reach = text.T("not being forwarded right now")
		}
		route.Rows = []row{
			{Text: text.T("Address"), Kind: "kv", Link: s.URL, LinkText: s.URL,
				Buttons: []button{
					quiet("copy:"+s.URL, text.T("Copy"), false),
					quiet(s.URL, text.T("Open"), false),
				}},
			{Text: text.T("Certificate"), Kind: "kv", Detail: s.Domain,
				Faint: certStatus(snap, s.Domain)},
		}
		route.Rows = append(route.Rows, row{Text: text.T("Forwarding"), Kind: "kv",
			Detail: dotWord(s.Routed), Faint: reach})
	}
	route.Rows = append(route.Rows, row{Text: text.T("Container port"), Kind: "kv",
		Detail: fmt.Sprint(s.Port),
		Faint:  text.T("read from the Compose file, no host port published")})
	p.Sections = append(p.Sections, route)

	container := section{Header: text.T("CONTAINER"), Rows: []row{
		{Text: text.T("Address"), Kind: "kv", Detail: orDash(s.IPv4), Mono: true},
		{Text: text.T("Image"), Kind: "kv", Detail: orDash(s.Image), Mono: true},
		{Text: text.T("Reachable as"), Kind: "kv", Detail: s.Container, Mono: true,
			Faint: text.T("from the other services in this project")},
	}}
	p.Sections = append(p.Sections, container)

	note := text.P("last %d line", "last %d lines", logTail, logTail)
	if s.State == "absent" {
		note = text.T("no container yet")
	}
	// The pane is part of the screen whether or not the container has written
	// anything, so an empty tail still says so.
	if len(log) == 0 {
		log = []string{text.T("(no output yet)")}
	}
	p.Sections = append(p.Sections, section{
		Header:  text.T("OUTPUT"),
		Note:    text.T("re-read every time this screen refreshes"),
		Log:     log,
		LogNote: note,
		LogButtons: []button{
			quiet("refresh", text.T("Refresh"), busy),
			quiet("copy-log:"+g.Name+":"+s.Name, text.T("Copy all"), s.State == "absent"),
			quiet("logs:"+g.Name+":"+s.Name, text.T("Open in a window"), s.State == "absent"),
		},
	})
}

func domainsView(p *panel, snap stack.Snapshot, busy bool) {
	p.Header = header{
		Title:    text.T("Domains"),
		Subtitle: text.T("delegated to bay on this machine"),
		Buttons: []button{{ID: "machine-domain-add", Title: text.T("Add domain…"),
			Disabled: busy}},
	}

	list := section{}
	for _, d := range snap.Machine.DomainList {
		r := row{Text: d.Name, Dot: "on"}
		switch {
		case d.Default:
			r.LinkText = text.T("projects without one of their own use it")
			r.Chip = text.T("default")
		case len(d.PinnedBy) > 0:
			r.LinkText = text.T("pinned by %s", strings.Join(d.PinnedBy, ", "))
		default:
			r.LinkText = text.T("delegated")
		}
		if !d.Default {
			r.Buttons = append(r.Buttons,
				quiet("do-machine-domain-default:"+d.Name, text.T("Make default"), busy))
		}
		r.Buttons = append(r.Buttons, quiet("machine-domain-remove:"+d.Name, text.T("Remove"),
			busy || d.Default || len(d.PinnedBy) > 0))
		list.Rows = append(list.Rows, r)
	}
	if len(list.Rows) == 0 {
		list.Note = text.T("No domains are delegated on this machine.")
	}
	p.Sections = append(p.Sections, list)

	p.Sections = append(p.Sections, section{
		Header: text.T("HOW IT WORKS"),
		Rows: []row{
			{Text: text.T("Resolver"), Kind: "kv", Mono: true,
				Detail: resolverPaths(snap.Machine.Domains) + " → " + snap.Machine.DNS.Addr},
			{Text: text.T("Adding one"), Kind: "kv",
				Detail: text.T("asks for your password once")},
			{Text: text.T("Pinned domains"), Kind: "kv",
				Detail: text.T("a domain named in a project's Compose file is removed there")},
		},
	})
}

func certificatesView(p *panel, snap stack.Snapshot, busy bool) {
	p.Header = header{
		Title:    text.T("Certificates"),
		Subtitle: text.T("issued by this machine's local authority"),
		Buttons: []button{
			quiet("cert-reissue-all", text.T("Reissue all"), busy),
			quiet("ca-rotate", text.T("Replace authority…"), busy),
		},
	}

	ca := snap.Certificates.Authority
	trust := text.T("trusted in your keychain")
	if !ca.Trusted {
		trust = text.T("not trusted")
	}
	authority := row{
		Text: "bay", Wide: true, Dot: dotFor(ca.Trusted && !ca.NeedsAttention()),
		LinkText: trust, Detail: certLife(ca),
	}
	// Trusting it is one of the setup steps, so the button appears only while
	// that step is outstanding.
	if !ca.Trusted {
		authority.Buttons = []button{hero("setup", text.T("Trust it"), busy)}
	}
	p.Sections = append(p.Sections, section{Header: text.T("AUTHORITY"), Rows: []row{authority}})

	// The certificates something asks for and the ones left behind are two
	// different lists: one is reissued, the other is removed.
	issued := section{Header: text.T("ISSUED")}
	unused := section{Header: text.T("UNUSED"),
		Note: text.T("no route and no project asks for these names")}
	for _, info := range snap.Certificates.Issued {
		r := row{
			Text: info.Name, Wide: true, Dot: dotFor(!info.NeedsAttention()),
			Detail: certLife(info),
		}
		if info.Orphaned {
			r.Buttons = []button{quiet("cert-remove:"+info.Name, text.T("Remove"), busy)}
			unused.Rows = append(unused.Rows, r)
			continue
		}
		r.LinkText = certOwner(snap, info)
		reissue := quiet("cert-reissue:"+info.Name, text.T("Reissue"), busy)
		if info.NeedsAttention() {
			reissue.Style = ""
		}
		r.Buttons = []button{reissue}
		issued.Rows = append(issued.Rows, r)
	}
	if len(issued.Rows) == 0 {
		issued.Note = text.T("Nothing issued yet. Certificates appear as routes do.")
	}
	p.Sections = append(p.Sections, issued)
	if len(unused.Rows) > 0 {
		unused.Buttons = []button{
			quiet("cert-remove-unused", text.T("Remove all %d", len(unused.Rows)), busy),
		}
		p.Sections = append(p.Sections, unused)
	}
}

func settingsView(p *panel, snap stack.Snapshot, busy bool) {
	p.Header = header{Title: text.T("Settings")}

	p.Sections = append(p.Sections, section{
		Header: text.T("APPLICATION"),
		Rows: []row{
			{Text: text.T("Appearance"), Kind: "kv", Appearance: true},
			{Text: text.T("Language"), Kind: "kv", Segment: &segment{
				ID: "language", Labels: languageLabels(),
				Selected: i18n.IndexOf(languageChoice())}},
			{Text: text.T("Window"), Kind: "kv", Detail: text.T("Show the window at launch"),
				Toggle: "toggle-show-at-launch", On: showAtLaunch()},
		},
	})

	dns := text.T("registered")
	switch {
	case !snap.Machine.DNS.Loaded:
		dns = text.T("not registered")
	case !snap.Machine.DNS.Current:
		dns = text.T("registered for other domains")
	}
	p.Sections = append(p.Sections, section{
		Header: text.T("MACHINE"),
		Rows: []row{
			{Text: text.T("Default domain"), Kind: "kv", Detail: defaultDomain(snap),
				Faint:   text.T("projects without one of their own use it"),
				Buttons: []button{quiet("machine-domain-default", text.T("Change…"), busy)}},
			{Text: text.T("DNS agent"), Kind: "kv", Detail: snap.Machine.DNS.Addr,
				Mono: true, Faint: dns},
			{Text: text.T("State"), Kind: "kv", Detail: shortPath(snap.Machine.StateDir),
				Mono:    true,
				Buttons: []button{quiet("reveal:"+snap.Machine.StateDir, text.T("Reveal"), false)}},
		},
	})

	p.Sections = append(p.Sections, section{
		Header: text.T("MAINTENANCE"),
		Rows: []row{
			{Text: text.T("Check setup"), Kind: "kv", Detail: text.T("Run doctor"),
				Hint:    text.T("Reports the machine setup and changes nothing."),
				Buttons: []button{quiet("doctor", text.T("Run"), busy)}},
			{Text: text.T("Proxy"), Kind: "kv",
				Detail:  text.T("Rewrite its configuration from what is running"),
				Hint:    text.T("Starts, stops and changes no container."),
				Buttons: []button{quiet("proxy-sync", text.T("Rewrite"), busy)}},
			{Text: text.T("Reinstall"), Kind: "kv",
				Detail:  text.T("Rewrite the resolver entries and re-trust the authority"),
				Hint:    text.T("Asks for your password once."),
				Buttons: []button{quiet("setup", text.T("Reinstall"), busy)}},
			{Text: text.T("Remove"), Kind: "kv",
				Detail:  text.T("Remove the resolver entries and the DNS agent"),
				Hint:    text.T("Names under the delegated domains stop resolving."),
				Buttons: []button{quiet("machine-uninstall", text.T("Remove…"), busy)}},
		},
	})
}

// resolverPaths names the files that delegate the domains. Several names share
// one directory, so they are written as a set: a list of whole paths does not
// fit the row and truncating it in the middle hides a name.
func resolverPaths(domains []string) string {
	switch len(domains) {
	case 0:
		return "/etc/resolver/"
	case 1:
		return "/etc/resolver/" + domains[0]
	default:
		return "/etc/resolver/{" + strings.Join(domains, ",") + "}"
	}
}

// defaultDomain returns the domain projects fall back to.
func defaultDomain(snap stack.Snapshot) string {
	for _, d := range snap.Machine.DomainList {
		if d.Default {
			return d.Name
		}
	}
	return stack.DefaultDomain
}

func certStatus(snap stack.Snapshot, domain string) string {
	for _, info := range snap.Certificates.Issued {
		if info.Name == domain {
			return certLife(info)
		}
	}
	return text.T("not issued yet")
}

// certLife states how long a certificate is good for. The command line prints
// the same fact in English through CertInfo.Status; the window composes it so
// it can be read in the window's language.
func certLife(info stack.CertInfo) string {
	switch {
	case info.Unreadable() != "":
		return text.T("unreadable: %s", info.Unreadable())
	case info.Expired():
		return text.T("expired")
	case info.DaysLeft() <= 30:
		return text.P("expires in %d day", "expires in %d days", info.DaysLeft(), info.DaysLeft())
	default:
		return text.T("valid until %s", info.NotAfter.Format("2006-01-02"))
	}
}

// unusedCertificates returns the names no route and no registered project asks
// for. It is read from the snapshot at the moment of removal rather than
// carried through the dialog, so a certificate that came into use while the
// question was open is kept.
func unusedCertificates(snap stack.Snapshot) []string {
	var names []string
	for _, info := range snap.Certificates.Issued {
		if info.Orphaned {
			names = append(names, info.Name)
		}
	}
	return names
}

// certOwner returns the project a certificate's domain belongs to.
func certOwner(snap stack.Snapshot, info stack.CertInfo) string {
	for _, g := range snap.Groups {
		for _, s := range g.Services {
			if s.Domain == info.Name {
				return g.Name
			}
		}
	}
	return ""
}

func routedCount(g stack.GroupStatus) int {
	n := 0
	for _, s := range g.Services {
		if s.Routed {
			n++
		}
	}
	return n
}

// serviceState describes one service for the detail header.
func serviceState(s stack.ServiceStatus) string {
	switch s.State {
	case "running":
		if d := s.Uptime(); d > 0 {
			return text.T("running for %s", humanDuration(d))
		}
		return text.T("running")
	case "starting":
		return text.T("running, not listening yet")
	case "absent":
		return text.T("no container")
	default:
		return s.State
	}
}

// humanDuration writes a duration the way the header reads it: hours and
// minutes, or minutes and seconds under an hour.
func humanDuration(d time.Duration) string {
	d = d.Round(time.Second)
	if d >= time.Hour {
		return text.T("%d h %d m", int(d/time.Hour), int(d%time.Hour/time.Minute))
	}
	if d >= time.Minute {
		return text.T("%d m %d s", int(d/time.Minute), int(d%time.Minute/time.Second))
	}
	return text.T("%d s", int(d/time.Second))
}

// shortPath writes a path under the home directory with a leading tilde.
func shortPath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || !strings.HasPrefix(p, home+string(filepath.Separator)) {
		return p
	}
	return "~" + strings.TrimPrefix(p, home)
}

func startTitle(missing int, all bool) string {
	if all {
		return text.T("Start all %d", missing)
	}
	return text.T("Start the other %d", missing)
}

func dotWord(ok bool) string {
	if ok {
		return text.T("yes")
	}
	return text.T("no")
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// sideDot and serviceDot return "off" rather than nothing, because a row with
// no dot loses its place in the column. Nothing running is not a fault, so it
// is neither amber nor red.
func sideDot(state groupState) string {
	switch state {
	case stateRunning:
		return "on"
	case statePartial:
		return "warn"
	default:
		return "off"
	}
}

func serviceDot(s stack.ServiceStatus) string {
	switch {
	case s.Running() && (s.Routed || s.Internal):
		return "on"
	case s.Live():
		return "warn"
	default:
		return "off"
	}
}

func dotFor(ok bool) string {
	if ok {
		return "on"
	}
	return "warn"
}
