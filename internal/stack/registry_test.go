package stack

import "testing"

func TestServiceInstanceBackendIsNamed(t *testing.T) {
	in := ServiceInstance{Container: "alpha-web", Port: 8080}
	if got, want := in.Backend(), "alpha-web."+BackendDomain+":8080"; got != want {
		t.Fatalf("Backend() = %q, want %q", got, want)
	}
}
