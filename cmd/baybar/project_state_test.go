package main

import (
	"testing"

	"github.com/polyspec/bay/internal/stack"
)

// soksakimHyper is the project as the machine reported it: four services
// running and accepting connections, three stopped.
func soksakimHyper() stack.GroupStatus {
	on := func(n string) stack.ServiceStatus {
		return stack.ServiceStatus{Name: n, State: "running", Internal: true, IPv4: "192.168.65.1"}
	}
	off := func(n string) stack.ServiceStatus {
		return stack.ServiceStatus{Name: n, State: "stopped", Internal: true}
	}
	return stack.GroupStatus{Name: "soksakim-hyper", Services: []stack.ServiceStatus{
		off("libraries"), on("node"), on("node-web"), on("php"), off("php-libraries"), on("php-web"), off("runtime")}}
}

// soksakimHyperTest is the project as the machine reported it: every container
// running, node not accepting connections on its port.
func soksakimHyperTest() stack.GroupStatus {
	on := func(n string) stack.ServiceStatus {
		return stack.ServiceStatus{Name: n, State: "running", Internal: true, IPv4: "192.168.65.1"}
	}
	node := on("node")
	node.State = "starting"
	return stack.GroupStatus{Name: "soksakim-hyper-test", Services: []stack.ServiceStatus{
		node, on("node-web"), on("php"), on("php-web")}}
}

// Stopped is not a fault, so a project with stopped services and nothing else
// wrong is not orange.
func TestAProjectWithStoppedServicesIsNotOrange(t *testing.T) {
	if got := projectDot(soksakimHyper()); got != "on" {
		t.Errorf("dot %q, want on: what runs works, and stopped is not a fault", got)
	}
}

// A running service not working as declared makes its project orange.
func TestAProjectWithAServiceNotWorkingIsOrange(t *testing.T) {
	if got := projectDot(soksakimHyperTest()); got != "warn" {
		t.Errorf("dot %q, want warn", got)
	}
}

// The count is the number of services whose container runs, and the button
// follows the same count.
func TestTheCountAndTheButtonFollowOneMeasure(t *testing.T) {
	for _, c := range []struct {
		g      stack.GroupStatus
		count  string
		button string
	}{
		{soksakimHyper(), text.T("%d of %d running", 4, 7), text.T("Start the rest")},
		{soksakimHyperTest(), text.T("%d of %d running", 4, 4), text.T("Stop")},
	} {
		p := panel{}
		dashboardView(&p, stack.Snapshot{Groups: []stack.GroupStatus{c.g}}, false)
		var got row
		for _, s := range p.Sections {
			for _, r := range s.Rows {
				if r.Text == c.g.Name {
					got = r
				}
			}
		}
		if got.Detail != c.count {
			t.Errorf("%s: count %q, want %q", c.g.Name, got.Detail, c.count)
		}
		if len(got.Buttons) == 0 || got.Buttons[len(got.Buttons)-1].Title != c.button {
			t.Errorf("%s: button %+v, want %q", c.g.Name, got.Buttons, c.button)
		}
	}
}

// The sidebar's count is the same count.
func TestTheSidebarCountsRunningContainers(t *testing.T) {
	snap := stack.Snapshot{Groups: []stack.GroupStatus{soksakimHyperTest()}}
	for _, g := range sidebar(snap, viewDashboard, nil) {
		for _, it := range g.Items {
			if it.Label == "soksakim-hyper-test" && it.Count != "4/4" {
				t.Errorf("sidebar count %q, want 4/4", it.Count)
			}
		}
	}
}
