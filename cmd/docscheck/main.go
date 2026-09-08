// Command docscheck verifies that the documents name only things the code
// declares, that their links resolve, and that every reader-facing document has
// a Korean twin.
//
// It cannot verify that a sentence is true. What it can verify is that a
// document does not refer to a command, a Compose key or a label the code no
// longer has, which is how these documents went stale before.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/polyspec/bay/internal/contract"
)

// Documents that are canonical English and require a `.ko.md` twin. Generated
// specification documents are excluded: they carry tables produced from the
// code, and a translated copy would go stale on every regeneration.
var needsTwin = []string{
	"README.md",
	"CHANGELOG.md",
	"docs/documentation-plan.md",
	"docs/features.md",
	"docs/operations/install.md",
	"docs/operations/using.md",
	"docs/operations/troubleshooting.md",
	"docs/spec/service-lifecycle.md",
	"docs/spec/volumes.md",
	"docs/spec/diagnostics.md",
}

var (
	linkPattern    = regexp.MustCompile(`\[[^\]]*\]\(([^)]+)\)`)
	headingPattern = regexp.MustCompile(`(?m)^#{1,6} `)
	commandPattern = regexp.MustCompile(`bay ([a-z][a-z-]*)`)
	keyPattern     = regexp.MustCompile(`x-bay\.([a-z_]+)`)
	// A label match must start the token and must not be followed by more of a
	// path: "x-bay.extra_domains" is a project key,
	// "dev.bay.dns" is a bundle identifier, and
	// "bay.git" ends a repository URL.
	labelPattern = regexp.MustCompile(`(^|[^\w.\-/])bay\.([a-z]+)`)
)

// flagWords appear after "bay" in prose but are not commands.
var flagWords = map[string]bool{
	"binary": true, "binaries": true, "brief": false,
}

func main() {
	var problems []string
	problems = append(problems, checkLinks()...)
	problems = append(problems, checkTwins()...)
	problems = append(problems, checkNames()...)
	problems = append(problems, checkEvidence()...)

	for _, p := range problems {
		fmt.Fprintln(os.Stderr, "docs-check: "+p)
	}
	if len(problems) > 0 {
		os.Exit(1)
	}
	fmt.Println("docs-check: ok")
}

// checkLinks reports relative Markdown links that point at a missing file.
func checkLinks() []string {
	var problems []string
	for _, path := range markdownFiles() {
		body, err := os.ReadFile(path)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		for _, m := range linkPattern.FindAllStringSubmatch(string(body), -1) {
			target := m[1]
			if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") ||
				strings.HasPrefix(target, "#") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			if i := strings.IndexByte(target, '#'); i >= 0 {
				target = target[:i]
			}
			if target == "" {
				continue
			}
			resolved := filepath.Join(filepath.Dir(path), target)
			if _, err := os.Stat(resolved); err != nil {
				problems = append(problems,
					fmt.Sprintf("%s links to %s, which does not exist", path, target))
			}
		}
	}
	return problems
}

// checkNames reports a document that names a command, a Compose key or a label
// the code does not declare.
func checkNames() []string {
	commands := map[string]bool{}
	for _, c := range contract.Commands() {
		commands[c.Name] = true
	}
	keys := map[string]bool{}
	for _, k := range contract.ProjectKeys() {
		if name, ok := strings.CutPrefix(k.Path, "x-bay."); ok {
			keys[name] = true
		}
	}
	labels := map[string]bool{}
	for _, set := range [][]contract.Key{contract.ServiceLabels(), contract.InternalLabels()} {
		for _, l := range set {
			if name, ok := strings.CutPrefix(l.Path, "bay."); ok {
				labels[name] = true
			}
		}
	}

	var problems []string
	for _, path := range markdownFiles() {
		body, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		text := string(body)
		for _, m := range commandPattern.FindAllStringSubmatch(text, -1) {
			word := m[1]
			if commands[word] || flagWords[word] {
				continue
			}
			// Only report a word that reads as a command: the sentence
			// "bay runs Compose projects" is prose, not a command.
			if isProse(word) {
				continue
			}
			problems = append(problems,
				fmt.Sprintf("%s names the command %q, which the code does not declare", path, word))
		}
		for _, m := range keyPattern.FindAllStringSubmatch(text, -1) {
			if !keys[m[1]] {
				problems = append(problems, fmt.Sprintf(
					"%s names x-bay.%s, which the code does not declare", path, m[1]))
			}
		}
		for _, m := range labelPattern.FindAllStringSubmatch(text, -1) {
			if m[2] == "git" {
				continue // a repository URL, not a label
			}
			if !labels[m[2]] {
				problems = append(problems, fmt.Sprintf(
					"%s names the label bay.%s, which the code does not declare", path, m[2]))
			}
		}
	}
	return problems
}

// isProse reports whether the word after "bay" is an English verb or
// article rather than a command name.
func isProse(word string) bool {
	switch word {
	case "runs", "install", "is", "was", "and", "or", "the", "a", "reads", "rejects",
		"binary", "binaries", "prints", "acquires", "registers", "creates":
		return true
	}
	return false
}

// checkEvidence reports a feature row whose evidence names no command and no
// test. An empty cell and a cell saying "done" are both rejected.
func checkEvidence() []string {
	body, err := os.ReadFile("docs/features.md")
	if err != nil {
		return []string{"docs/features.md is missing"}
	}
	var problems []string
	rows := 0
	for _, line := range strings.Split(string(body), "\n") {
		if !strings.HasPrefix(line, "| ") || strings.HasPrefix(line, "| ---") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		if len(cells) < 4 {
			continue
		}
		name := strings.TrimSpace(cells[0])
		if name == "" || name == "Feature" {
			continue
		}
		rows++
		status := strings.TrimSpace(cells[1])
		evidence := strings.TrimSpace(cells[2])
		shipped := strings.TrimSpace(cells[3])
		if status == "" || shipped == "" {
			problems = append(problems, fmt.Sprintf("feature %q has no status or shipped state", name))
		}
		if status == "Not started" {
			continue
		}
		// Evidence names the command that was run, written in backticks. A
		// sentence without one describes an intention, not a verification.
		if !strings.Contains(evidence, "`") {
			problems = append(problems, fmt.Sprintf(
				"feature %q states no command that was run: %q", name, evidence))
		}
	}
	if rows == 0 {
		problems = append(problems, "docs/features.md lists no features")
	}
	return problems
}

// checkTwins reports a missing Korean document, or one whose heading count
// differs from the English document. It does not compare the text.
func checkTwins() []string {
	var problems []string
	for _, path := range needsTwin {
		twin := strings.TrimSuffix(path, ".md") + ".ko.md"
		english, err := os.ReadFile(path)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s is missing", path))
			continue
		}
		korean, err := os.ReadFile(twin)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s has no Korean document at %s", path, twin))
			continue
		}
		e := len(headingPattern.FindAllString(string(english), -1))
		k := len(headingPattern.FindAllString(string(korean), -1))
		if e != k {
			problems = append(problems, fmt.Sprintf(
				"%s has %d headings and %s has %d", path, e, twin, k))
		}
	}
	return problems
}

func markdownFiles() []string {
	var out []string
	roots := []string{".", "docs"}
	seen := map[string]bool{}
	for _, root := range roots {
		filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".md") || seen[path] {
				return nil
			}
			if strings.HasPrefix(path, "design/") || strings.Contains(path, "/node_modules/") {
				return nil
			}
			seen[path] = true
			out = append(out, path)
			return nil
		})
	}
	return out
}
