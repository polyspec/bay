package stack

import (
	"fmt"
	"sync"
	"testing"
)

// Projects registered at the same time are all registered: each rewrite of the
// registry used to read it, and the last writer dropped the others' projects,
// and writers took each other's temporary file.
func TestConcurrentRegistrationsAreAllKept(t *testing.T) {
	m := NewMachine(t.TempDir())
	const n = 24
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- m.Register(GroupRef{Name: fmt.Sprintf("p%02d", i)})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	groups, err := m.Groups()
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != n {
		t.Fatalf("%d of %d projects registered", len(groups), n)
	}
}
