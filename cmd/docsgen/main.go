// Command docsgen writes the generated sections of the specification documents
// and checks that they match the code. The generated text comes from
// internal/contract, so a document cannot state a different contract than the
// implementation.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/polyspec/bay/internal/contract"
)

// A generated section starts with beginMarker and ends with endMarker. The
// text between them is replaced.
const (
	beginMarker = "<!-- generated:"
	endMarker   = "<!-- end generated -->"
)

// A target with Whole set is generated as a whole file; the others have a
// generated section.
var targets = []struct {
	Path   string
	Render func(io.Writer)
	Whole  bool
}{
	{"docs/spec/cli.md", contract.MarkdownCLI, false},
	{"docs/spec/compose-schema.md", contract.MarkdownSchema, false},
	{"README.md", contract.MarkdownAgents, false},
	{"README.ko.md", contract.MarkdownAgents, false},
	// The skill is committed so an agent installer that reads a repository
	// finds it; "bay skill install" writes the same bytes.
	{"skills/bay/SKILL.md", contract.Skill, true},
}

func main() {
	write := flag.Bool("write", false, "write the generated sections instead of checking them")
	flag.Parse()

	failed := false
	for _, t := range targets {
		current, err := os.ReadFile(t.Path)
		if t.Whole && os.IsNotExist(err) {
			current, err = nil, nil
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "docsgen: %v\n", err)
			failed = true
			continue
		}
		var body bytes.Buffer
		t.Render(&body)
		next, err := body.String(), error(nil)
		if !t.Whole {
			next, err = replace(string(current), body.String())
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "docsgen: %s: %v\n", t.Path, err)
			failed = true
			continue
		}
		if next == string(current) {
			continue
		}
		if !*write {
			fmt.Fprintf(os.Stderr,
				"docsgen: %s does not match the code; run \"make docs-generate\"\n", t.Path)
			failed = true
			continue
		}
		if err := os.MkdirAll(filepath.Dir(t.Path), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "docsgen: %v\n", err)
			failed = true
			continue
		}
		if err := os.WriteFile(t.Path, []byte(next), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "docsgen: %v\n", err)
			failed = true
			continue
		}
		fmt.Println("updated", t.Path)
	}
	if failed {
		os.Exit(1)
	}
}

// replace puts body between the markers, keeping the marker lines.
func replace(doc, body string) (string, error) {
	lines := strings.Split(doc, "\n")
	start, end := -1, -1
	for i, l := range lines {
		switch {
		case strings.HasPrefix(l, beginMarker) && start < 0:
			start = i
		case l == endMarker && start >= 0 && end < 0:
			end = i
		}
	}
	if start < 0 || end < 0 {
		return "", fmt.Errorf("no generated section")
	}
	out := append([]string{}, lines[:start+1]...)
	out = append(out, "")
	out = append(out, strings.Split(strings.TrimRight(body, "\n"), "\n")...)
	out = append(out, "")
	out = append(out, lines[end:]...)
	return strings.Join(out, "\n"), nil
}
