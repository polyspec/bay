package main

import (
	"fmt"
	"strings"

	"github.com/polyspec/bay/internal/stack"
)

// The window is a source list: navigation on the left and one subject on the
// right. The first item is a dashboard, which shows the whole machine, because
// the source list displays only the selected item.

// Selections the sidebar can hold. A project is "project:<name>".
const (
	viewDashboard = "dash"
	viewDomains   = "domains"
	viewCerts     = "certs"
	viewProject   = "project:"
)

// groupState is a project's run state. A running count is shown only for a
// partly running project; most projects on a machine are stopped, and a count
// on those would suggest a fault.
type groupState int

const (
	stateStopped groupState = iota
	statePartial
	stateRunning
)

func stateOf(g stack.GroupStatus) (groupState, int) {
	running := 0
	for _, s := range g.Services {
		if s.Live() {
			running++
		}
	}
	switch {
	case running == 0:
		return stateStopped, 0
	case running == len(g.Services):
		return stateRunning, running
	default:
		return statePartial, running
	}
}

func groupLine(g stack.GroupStatus) string {
	state, running := stateOf(g)
	switch state {
	case stateStopped:
		return g.Name + " - stopped"
	case statePartial:
		return fmt.Sprintf("%s - %d of %d running", g.Name, running, len(g.Services))
	default:
		return g.Name + " - running"
	}
}

// machineLine returns the machine's status line and whether setup is
// incomplete.
func machineLine(snap stack.Snapshot) (string, bool) {
	if n := len(snap.Machine.Pending); n > 0 {
		return fmt.Sprintf("Finish setup (%d step%s, asks for your password)", n, plural(n)), true
	}
	p := snap.Machine.Proxy
	if p.State != "running" {
		return "Nothing is running", false
	}
	return fmt.Sprintf("Serving %d domain%s", p.Routes, plural(p.Routes)), false
}

// iconFor returns the menu bar icon state. A stopped project is not reported as
// a fault.
func iconFor(snap stack.Snapshot) fill {
	if snap.Machine.Proxy.State != "running" {
		return fillNone
	}
	if len(snap.Machine.Pending) > 0 {
		return fillHalf
	}
	for _, g := range snap.Groups {
		// A partly running project differs from its Compose file.
		if state, _ := stateOf(g); state == statePartial {
			return fillHalf
		}
	}
	return fillFull
}

// buildPanel turns the snapshot into the window. busy disables every control,
// because an action already changing containers should not be raced.
func buildPanel(snap stack.Snapshot, busy bool, selected, message, kind string) panel {
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

func sidebar(snap stack.Snapshot, selected string) []sideGroup {
	machine := sideGroup{Title: "MACHINE", Items: []sideItem{
		{ID: "select:" + viewDashboard, Label: "Dashboard",
			Dot: dotFor(len(snap.Machine.Pending) == 0), Selected: selected == viewDashboard},
		{ID: "select:" + viewDomains, Label: "Domains",
			Dot: "on", Count: fmt.Sprint(len(snap.Machine.DomainList)),
			Selected: selected == viewDomains},
		{ID: "select:" + viewCerts, Label: "Certificates",
			Dot:   dotFor(snap.Machine.CATrusted),
			Count: fmt.Sprint(len(snap.Certificates.Issued)), Selected: selected == viewCerts},
	}}

	projects := sideGroup{Title: "PROJECTS"}
	for _, g := range snap.Groups {
		state, running := stateOf(g)
		dot := ""
		switch state {
		case stateRunning:
			dot = "on"
		case statePartial:
			dot = "warn"
		}
		projects.Items = append(projects.Items, sideItem{
			ID:       "select:" + viewProject + g.Name,
			Label:    g.Name,
			Dot:      dot,
			Count:    fmt.Sprintf("%d/%d", running, len(g.Services)),
			Selected: selected == viewProject+g.Name,
		})
	}
	if len(projects.Items) == 0 {
		return []sideGroup{machine}
	}
	return []sideGroup{machine, projects}
}

func dashboardView(p *panel, snap stack.Snapshot, busy bool) {
	p.Header = header{Title: "Dashboard"}

	line, needsSetup := machineLine(snap)
	running, total := 0, 0
	for _, g := range snap.Groups {
		if state, _ := stateOf(g); state != stateStopped {
			running++
		}
		total += len(g.Services)
	}
	sub := fmt.Sprintf("%d of %d project%s running · %d service%s",
		running, len(snap.Groups), plural(len(snap.Groups)), total, plural(total))
	if ip := snap.Machine.Proxy.IPv4; ip != "" {
		sub += " · proxy " + ip
	}
	p.Verdict = &verdict{Dot: dotFor(!needsSetup && snap.Machine.Proxy.State == "running"),
		Headline: line, Subline: sub}

	if needsSetup {
		p.Verdict.Dot = "warn"
		p.Banner = &banner{
			Title:  "This machine cannot serve the domains yet",
			Text:   strings.Join(snap.Machine.Pending, "\n") + "\nIt asks for your password once.",
			Button: &button{ID: "setup", Title: "Finish setup", Primary: true, Disabled: busy},
		}
	}

	projects := section{Header: "PROJECTS"}
	for _, g := range snap.Groups {
		state, running := stateOf(g)
		detail := "stopped"
		action := button{ID: "up:" + g.Name, Title: "Start", Primary: true, Disabled: busy}
		switch state {
		case stateRunning:
			detail = fmt.Sprintf("%d of %d running", running, len(g.Services))
			action = button{ID: "down:" + g.Name, Title: "Stop", Disabled: busy}
		case statePartial:
			detail = fmt.Sprintf("%d of %d running", running, len(g.Services))
			action = button{ID: "up:" + g.Name, Title: "Start the rest", Primary: true, Disabled: busy}
		}
		dots := make([]string, 0, len(g.Services))
		for _, s := range g.Services {
			dot := ""
			switch {
			case s.Running():
				dot = "on"
			case s.Live():
				dot = "warn"
			}
			dots = append(dots, dot)
		}
		projects.Rows = append(projects.Rows, row{
			Text: g.Name, Dot: sideDot(state), Chip: g.Domain, Dots: dots,
			Detail: detail, Buttons: []button{action},
			ID: "select:" + viewProject + g.Name,
		})
	}
	if len(projects.Rows) == 0 {
		projects.Note = "No projects yet. Run \"bay up\" in a project directory."
	}
	p.Sections = append(p.Sections, projects)

	addrs := section{Header: "ADDRESSES", Note: "everything the proxy is serving right now"}
	for _, g := range snap.Groups {
		for _, s := range g.Services {
			if !s.Routed {
				continue
			}
			addrs.Rows = append(addrs.Rows, row{
				Text: s.Name, Dot: "on", Link: s.URL, LinkText: s.URL, Detail: g.Name,
			})
		}
	}
	if len(addrs.Rows) > 0 {
		p.Sections = append(p.Sections, addrs)
	}
}

func projectView(p *panel, g stack.GroupStatus, busy bool) {
	state, running := stateOf(g)
	p.Header = header{Title: g.Name, Subtitle: g.Domain + " · " + g.StackPath}
	switch state {
	case stateStopped:
		p.Header.Buttons = []button{
			{ID: "up:" + g.Name, Title: fmt.Sprintf("Start all %d", len(g.Services)),
				Primary: true, Disabled: busy},
		}
	case statePartial:
		p.Header.Buttons = []button{
			{ID: "up:" + g.Name, Title: fmt.Sprintf("Start the other %d", len(g.Services)-running),
				Primary: true, Disabled: busy},
			{ID: "down:" + g.Name, Title: "Stop", Disabled: busy},
		}
	default:
		p.Header.Buttons = []button{
			{ID: "restart:" + g.Name, Title: "Restart all", Disabled: busy},
			{ID: "down:" + g.Name, Title: "Stop", Disabled: busy},
		}
	}

	services := section{Header: "SERVICES"}
	if g.Error != "" {
		services.Note = g.Error
	}
	for _, s := range g.Services {
		// The address column includes the scheme for both routed and internal
		// services.
		link, linkText := s.URL, s.URL
		detail := s.IPv4
		if detail == "" {
			detail = s.State
		}
		if s.Internal {
			link, linkText = "", "tcp://"+s.Address
		} else if s.Running() && !s.Routed {
			detail += " · not routed"
		}
		services.Rows = append(services.Rows, row{
			Text: s.Name, Dot: serviceDot(s), Link: link, LinkText: linkText, Detail: detail,
			Buttons: []button{
				{ID: "start:" + g.Name + ":" + s.Name, Title: "Start", Disabled: busy || s.Live()},
				{ID: "stop:" + g.Name + ":" + s.Name, Title: "Stop", Disabled: busy || !s.Live()},
				{ID: "logs:" + g.Name + ":" + s.Name, Title: "Logs", Disabled: s.State == "absent"},
			},
		})
	}
	p.Sections = append(p.Sections, services)

	pinned := "inherited from the machine"
	if g.DomainPinned {
		pinned = "pinned in this project's Compose file"
	}
	p.Sections = append(p.Sections, section{
		Header: "PROJECT",
		Rows: []row{
			{Text: "Domain", Kind: "kv", Detail: g.Domain + " · " + pinned,
				Buttons: []button{{ID: "domain-rename:" + g.Name + ":" + g.Domain,
					Title: "Use another…", Disabled: busy}}},
			{Text: "Compose file", Kind: "kv", Detail: g.StackPath},
			{Text: "Routes", Kind: "kv", Detail: fmt.Sprintf("%d of %d services", running, len(g.Services))},
		},
	})
}

func domainsView(p *panel, snap stack.Snapshot, busy bool) {
	p.Header = header{
		Title:    "Domains",
		Subtitle: "delegated to bay on this machine",
		Buttons:  []button{{ID: "machine-domain-add", Title: "Add domain…", Primary: true, Disabled: busy}},
	}

	list := section{}
	for _, d := range snap.Machine.DomainList {
		r := row{Text: d.Name, Dot: "on"}
		switch {
		case d.Default:
			r.LinkText = "projects without one of their own use it"
			r.Chip = "default"
		case len(d.PinnedBy) > 0:
			r.LinkText = "pinned by " + strings.Join(d.PinnedBy, ", ")
		default:
			r.LinkText = "delegated"
		}
		if !d.Default {
			r.Buttons = append(r.Buttons, button{
				ID: "do-machine-domain-default:" + d.Name, Title: "Make default", Disabled: busy})
		}
		r.Buttons = append(r.Buttons, button{
			ID:       "machine-domain-remove:" + d.Name,
			Title:    "Remove",
			Disabled: busy || d.Default || len(d.PinnedBy) > 0,
		})
		list.Rows = append(list.Rows, r)
	}
	if len(list.Rows) == 0 {
		list.Note = "No domains are delegated on this machine."
	}
	p.Sections = append(p.Sections, list)

	resolvers := make([]string, 0, len(snap.Machine.Domains))
	for _, d := range snap.Machine.Domains {
		resolvers = append(resolvers, "/etc/resolver/"+d)
	}
	p.Sections = append(p.Sections, section{
		Header: "HOW IT WORKS",
		Rows: []row{
			{Text: "Resolver", Kind: "kv",
				Detail: strings.Join(resolvers, ", ") + " → " + snap.Machine.DNS.Addr},
			{Text: "Adding one", Kind: "kv", Detail: "asks for your password once"},
			{Text: "Pinned domains", Kind: "kv",
				Detail: "a domain named in a project's Compose file is removed there"},
		},
	})
}

func certificatesView(p *panel, snap stack.Snapshot, busy bool) {
	p.Header = header{
		Title:    "Certificates",
		Subtitle: "issued by this machine's local authority",
		Buttons: []button{
			{ID: "cert-reissue-all", Title: "Reissue all", Disabled: busy},
			{ID: "ca-rotate", Title: "Replace authority…", Disabled: busy},
		},
	}

	ca := snap.Certificates.Authority
	trust := "trusted in your keychain"
	if !ca.Trusted {
		trust = "not trusted"
	}
	p.Sections = append(p.Sections, section{
		Header: "AUTHORITY",
		Rows: []row{{
			Text: "bay", Dot: dotFor(ca.Trusted && !ca.NeedsAttention()),
			LinkText: trust, Detail: ca.Status(),
			Buttons: []button{{ID: "setup", Title: "Trust it", Primary: true,
				Disabled: busy || ca.Trusted}},
		}},
	})

	issued := section{Header: "ISSUED"}
	for _, info := range snap.Certificates.Issued {
		where := ""
		if info.Orphaned {
			where = "no route uses it"
		}
		issued.Rows = append(issued.Rows, row{
			Text: info.Name, Dot: dotFor(!info.NeedsAttention()),
			LinkText: where, Detail: info.Status(),
			Buttons: []button{
				{ID: "cert-reissue:" + info.Name, Title: "Reissue", Disabled: busy},
				{ID: "cert-remove:" + info.Name, Title: "Remove", Disabled: busy},
			},
		})
	}
	if len(issued.Rows) == 0 {
		issued.Note = "Nothing issued yet. Certificates appear as routes do."
	}
	p.Sections = append(p.Sections, issued)
}

func sideDot(state groupState) string {
	switch state {
	case stateRunning:
		return "on"
	case statePartial:
		return "warn"
	default:
		return ""
	}
}

func serviceDot(s stack.ServiceStatus) string {
	switch {
	case s.Running() && (s.Routed || s.Internal):
		return "on"
	case s.Live():
		return "warn"
	default:
		return ""
	}
}

func dotFor(ok bool) string {
	if ok {
		return "on"
	}
	return "warn"
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
