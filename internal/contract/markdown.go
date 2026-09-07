package contract

import (
	"fmt"
	"io"
)

// The specification documents embed these tables between generated markers, so
// the documents and the code state the same contract.

// MarkdownCLI writes the command contract for docs/spec/cli.md.
func MarkdownCLI(w io.Writer) {
	fmt.Fprintln(w, "| Command | Summary | Administrator rights |")
	fmt.Fprintln(w, "| --- | --- | --- |")
	for _, c := range Commands() {
		name := c.Name
		if c.Args != "" {
			name += " " + c.Args
		}
		root := "No"
		if c.Root {
			root = "Can require"
		}
		fmt.Fprintf(w, "| `%s` | %s | %s |\n", name, c.Summary, root)
	}

	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "## Effects")
	for _, c := range Commands() {
		if c.Effect == "" {
			continue
		}
		fmt.Fprintf(w, "\n### `%s`\n\n%s\n", c.Name, c.Effect)
	}

	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "## Rules callers can rely on")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "| Rule | Reason |")
	fmt.Fprintln(w, "| --- | --- |")
	for _, i := range Invariants() {
		fmt.Fprintf(w, "| %s | %s |\n", i.Rule, i.Reason)
	}

	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "## Diagnostics")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "| Symptom | Cause | Command |")
	fmt.Fprintln(w, "| --- | --- | --- |")
	for _, d := range Diagnostics() {
		fmt.Fprintf(w, "| %s | %s | `%s` |\n", d.Symptom, d.Cause, d.Command)
	}
}

// MarkdownSchema writes the Compose contract for docs/spec/compose-schema.md.
func MarkdownSchema(w io.Writer) {
	fmt.Fprintln(w, "Compose files this tool reads, in search order:")
	fmt.Fprintln(w, "`compose.yaml`, `compose.yml`, `docker-compose.yaml`, `docker-compose.yml`.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Keys this tool adds are Compose extensions, so the same file stays")
	fmt.Fprintln(w, "readable by other Compose tools.")

	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "## Project keys")
	markdownKeys(w, ProjectKeys())

	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "## Service labels")
	markdownKeys(w, ServiceLabels())

	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "## Port resolution")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "First match wins.")
	fmt.Fprintln(w, "")
	for i, p := range PortOrder() {
		fmt.Fprintf(w, "%d. %s\n", i+1, p)
	}

	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "## Standard Compose keys applied")
	markdownKeys(w, ComposeKeys())

	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "## Example")
	fmt.Fprintf(w, "\n```yaml\n%s```\n", Example())
}

func markdownKeys(w io.Writer, keys []Key) {
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "| Key | Type | Default | Description |")
	fmt.Fprintln(w, "| --- | --- | --- | --- |")
	for _, k := range keys {
		fmt.Fprintf(w, "| `%s` | %s | %s | %s |\n", k.Path, k.Type, k.Default, k.Description)
	}
}
