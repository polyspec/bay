package main

import (
	"testing"

	"github.com/polyspec/bay/internal/stack"
)

func project(name string, services ...string) stack.GroupStatus {
	g := stack.GroupStatus{Name: name}
	for _, s := range services {
		g.Services = append(g.Services, stack.ServiceStatus{Name: s})
	}
	return g
}

// listed returns the labels of the sidebar's project rows, services included.
func listed(snap stack.Snapshot, selected string, open map[string]bool) []string {
	var out []string
	for _, g := range sidebar(snap, selected, open) {
		if g.Title != text.T("PROJECTS") {
			continue
		}
		for _, it := range g.Items {
			out = append(out, it.Label)
		}
	}
	return out
}

// Pressing a project that is not selected selects it and opens it.
func TestPressingAnUnselectedProjectOpensIt(t *testing.T) {
	open := nextOpen(map[string]bool{}, viewDashboard, viewProject+"a")
	if !open["a"] {
		t.Error("the pressed project is not open")
	}
}

// Pressing the selected project opens it or closes it.
func TestPressingTheSelectedProjectTogglesIt(t *testing.T) {
	open := nextOpen(map[string]bool{"a": true}, viewProject+"a", viewProject+"a")
	if open["a"] {
		t.Fatal("pressing the selected open project did not close it")
	}
	open = nextOpen(open, viewProject+"a", viewProject+"a")
	if !open["a"] {
		t.Error("pressing the selected closed project did not open it")
	}
}

// Selecting a service opens its project.
func TestSelectingAServiceOpensItsProject(t *testing.T) {
	open := nextOpen(map[string]bool{}, viewDashboard, viewService+"a:web")
	if !open["a"] {
		t.Error("the service's project is not open")
	}
}

// A project stays open when another is selected, so several can be open.
func TestOpenProjectsStayOpenWhenAnotherIsSelected(t *testing.T) {
	open := nextOpen(map[string]bool{"a": true}, viewProject+"a", viewProject+"b")
	if !open["a"] || !open["b"] {
		t.Errorf("open = %v, want both a and b", open)
	}
	snap := stack.Snapshot{Groups: []stack.GroupStatus{project("a", "web", "db"), project("b", "api"), project("c", "x")}}
	got := listed(snap, viewProject+"b", open)
	want := []string{"a", "web", "db", "b", "api", "c"}
	if len(got) != len(want) {
		t.Fatalf("rows %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rows %v, want %v", got, want)
		}
	}
}

// A closed project lists no services even while it is selected.
func TestAClosedSelectedProjectListsNoServices(t *testing.T) {
	snap := stack.Snapshot{Groups: []stack.GroupStatus{project("a", "web")}}
	got := listed(snap, viewProject+"a", map[string]bool{})
	if len(got) != 1 || got[0] != "a" {
		t.Errorf("rows %v, want only the project", got)
	}
}
