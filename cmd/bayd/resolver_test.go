package main

import (
	"errors"
	"testing"
	"time"

	"github.com/polyspec/bay/internal/stack"
)

// staleResolver returns a resolver that holds one list, older than the TTL, and
// whose next list is the function next.
func staleResolver(t *testing.T, next func() ([]stack.Instance, error)) (*resolver, *index) {
	t.Helper()
	r := &resolver{domains: []string{"test"}, list: func() ([]stack.Instance, error) { return nil, nil }}
	first, err := r.index()
	if err != nil {
		t.Fatalf("first list: %v", err)
	}
	r.mu.Lock()
	r.cached = time.Now().Add(-time.Hour)
	r.list = next
	r.mu.Unlock()
	return r, first
}

// TestIndexAnswersFromLastListWhileNewListRuns covers rule 11: a query does not
// wait for a new list once one list exists.
func TestIndexAnswersFromLastListWhileNewListRuns(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	r, first := staleResolver(t, func() ([]stack.Instance, error) {
		<-release
		return nil, nil
	})
	got := make(chan *index, 1)
	go func() {
		idx, err := r.index()
		if err != nil {
			t.Errorf("index: %v", err)
		}
		got <- idx
	}()
	select {
	case idx := <-got:
		if idx != first {
			t.Fatalf("index returned another list than the last one")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("index waited for the new list for more than 100 ms")
	}
}

// TestRefreshKeepsLastListWhenNewListFails covers rule 11: a failed new list
// keeps the last list.
func TestRefreshKeepsLastListWhenNewListFails(t *testing.T) {
	r, first := staleResolver(t, nil)
	r.mu.Lock()
	r.refreshing = true
	cached := r.cached
	r.mu.Unlock()
	r.refresh(func() ([]stack.Instance, error) { return nil, errors.New("container ls failed") })
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cache != first || !r.cached.Equal(cached) || r.refreshing {
		t.Fatalf("after a failed list: cache %p (want %p), cached %v (want %v), refreshing %v", r.cache, first, r.cached, cached, r.refreshing)
	}
}

// TestRefreshReplacesLastList covers rule 11: a new list replaces the last one.
func TestRefreshReplacesLastList(t *testing.T) {
	r, first := staleResolver(t, nil)
	r.mu.Lock()
	r.refreshing = true
	r.mu.Unlock()
	r.refresh(func() ([]stack.Instance, error) { return nil, nil })
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cache == first || r.refreshing || time.Since(r.cached) > time.Minute {
		t.Fatalf("after a new list: cache %p (the last was %p), refreshing %v, cached %v", r.cache, first, r.refreshing, r.cached)
	}
}
