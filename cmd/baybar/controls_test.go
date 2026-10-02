package main

import (
	"strings"
	"testing"

	"github.com/polyspec/bay/internal/stack"
)

func runningProject(name string) stack.GroupStatus {
	return stack.GroupStatus{Name: name, Services: []stack.ServiceStatus{{Name: "comparison", State: "running", IPv4: "192.168.65.2", URLs: []string{"https://crudui.test/"}}}}
}

// everyButton returns every button a panel holds, in its header and its rows.
func everyButton(p panel) []button {
	out := append([]button{}, p.Header.Buttons...)
	for _, s := range p.Sections {
		for _, r := range s.Rows {
			out = append(out, r.Buttons...)
		}
	}
	return out
}

// A control names what it does. The project's Stop sent down, and pressing it
// on crudui removed the container and the registration.
func TestNoStopButtonRemovesAnything(t *testing.T) {
	g := runningProject("crudui")
	snap := stack.Snapshot{Groups: []stack.GroupStatus{g}}
	for _, view := range []string{viewDashboard, viewProject + "crudui"} {
		p := buildPanel(snap, false, view, nil, "", "", nil, nil, nil)
		for _, b := range everyButton(p) {
			if b.Title == text.T("Stop") && strings.HasPrefix(b.ID, "down:") {
				t.Errorf("%s: a button titled Stop sends %q, which removes the containers", view, b.ID)
			}
		}
	}
}

func TestStopStopsTheProjectAndRemoveAsksFirst(t *testing.T) {
	g := runningProject("crudui")
	snap := stack.Snapshot{Groups: []stack.GroupStatus{g}}
	p := buildPanel(snap, false, viewProject+"crudui", nil, "", "", nil, nil, nil)
	ids := map[string]string{}
	for _, b := range p.Header.Buttons {
		ids[b.Title] = b.ID
	}
	if ids[text.T("Stop")] != "stop:crudui" {
		t.Errorf("Stop sends %q, want stop:crudui", ids[text.T("Stop")])
	}
	// Remove goes through a question, so the button does not send down itself.
	if ids[text.T("Remove")] != "project-remove:crudui" {
		t.Errorf("Remove sends %q, want project-remove:crudui", ids[text.T("Remove")])
	}
}

// An outcome states what was done to what. A step's own line is not one.
func TestTheOutcomeNamesTheActionAndItsSubject(t *testing.T) {
	for parts, want := range map[string]string{
		"stop:crudui":        text.T("Stopped %s", "crudui"),
		"down:crudui":        text.T("Removed %s", "crudui"),
		"up:crudui":          text.T("Started %s", "crudui"),
		"restart:crudui":     text.T("Restarted %s", "crudui"),
		"stop:crudui:web":    text.T("Stopped %s", "web"),
		"start:crudui:web":   text.T("Started %s", "web"),
		"restart:crudui:web": text.T("Restarted %s", "web"),
	} {
		got, ok := lifecycleOutcome(strings.Split(parts, ":"))
		if !ok || got != want {
			t.Errorf("%s: outcome %q, want %q", parts, got, want)
		}
	}
	if _, ok := lifecycleOutcome([]string{"peer-open"}); ok {
		t.Error("an action that is not a lifecycle action was given a lifecycle outcome")
	}
}

// When an action removes the subject of its screen, the dashboard is shown and
// the outcome moves there with it.
func TestARemovedProjectsScreenGivesWayToTheDashboard(t *testing.T) {
	snap := stack.Snapshot{Groups: []stack.GroupStatus{runningProject("other")}}
	for _, gone := range []string{viewProject + "crudui", viewService + "crudui:web"} {
		sel, at := resolveSelection(snap, gone, gone)
		if sel != viewDashboard || at != viewDashboard {
			t.Errorf("%s: selection %q, message at %q; want the dashboard for both", gone, sel, at)
		}
	}
	sel, at := resolveSelection(snap, viewProject+"other", viewDashboard)
	if sel != viewProject+"other" || at != viewDashboard {
		t.Errorf("a project that exists changed: %q, %q", sel, at)
	}
}
