package contract

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Brief writes the usage contract. A caller that installed only the binaries
// reads it instead of the repository's documents.
func Brief(w io.Writer) {
	fmt.Fprintf(w, "%s\n\n", wrap(Purpose, 78))

	fmt.Fprintln(w, "COMMANDS")
	CommandList(w)

	fmt.Fprintln(w, "\nCOMPOSE FILE")
	fmt.Fprintln(w, "  A project is a Compose file: compose.yaml, compose.yml,")
	fmt.Fprintln(w, "  docker-compose.yaml or docker-compose.yml. Settings live under the")
	fmt.Fprintln(w, "  top-level x-bay mapping and in bay.* service labels.")
	fmt.Fprintln(w, "  A service with no label is served at <service>.<project domain>.")
	fmt.Fprintln(w, "\n  Run \"bay schema\" for the full key list.")

	fmt.Fprintln(w, "\nSERVICE TO SERVICE")
	fmt.Fprintln(w, "  Reach another service at <project>-<service>.container.test:<port>.")
	fmt.Fprintln(w, "  Container addresses change on every start, so use the name.")

	fmt.Fprintln(w, "\nRULES")
	for _, i := range Invariants() {
		fmt.Fprintf(w, "  - %s\n", i.Rule)
	}

	fmt.Fprintln(w, "\nDIAGNOSTICS")
	width := 0
	for _, d := range Diagnostics() {
		if len(d.Symptom) > width {
			width = len(d.Symptom)
		}
	}
	for _, d := range Diagnostics() {
		fmt.Fprintf(w, "  %-*s  %s\n", width, d.Symptom, d.Command)
	}

	fmt.Fprintln(w, "\nMACHINE STATE")
	for _, s := range StateFiles() {
		if s.Root {
			fmt.Fprintf(w, "  %-48s  requires administrator rights\n", s.Path)
			continue
		}
		fmt.Fprintf(w, "  %s\n", s.Path)
	}
	fmt.Fprintln(w, "\n  Read the current state with \"bay status --json\".")
}

// BriefJSON writes the same contract as JSON.
func BriefJSON(w io.Writer) error {
	doc := map[string]any{
		"purpose":       Purpose,
		"commands":      Commands(),
		"projectKeys":   ProjectKeys(),
		"serviceLabels": ServiceLabels(),
		"composeKeys":   ComposeKeys(),
		"portOrder":     PortOrder(),
		"invariants":    Invariants(),
		"diagnostics":   Diagnostics(),
		"stateFiles":    StateFiles(),
		"composeFileNames": []string{
			"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml",
		},
		"serviceAddress": "<project>-<service>.container.test:<port>",
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// Schema writes the Compose extension contract.
func Schema(w io.Writer) {
	fmt.Fprintln(w, "Compose files this tool reads:")
	fmt.Fprintln(w, "  compose.yaml, compose.yml, docker-compose.yaml, docker-compose.yml")
	fmt.Fprintln(w, "\nKeys this tool adds are Compose extensions, so the same file remains")
	fmt.Fprintln(w, "readable by other Compose tools.")

	fmt.Fprintln(w, "\nPROJECT KEYS")
	writeKeys(w, ProjectKeys())

	fmt.Fprintln(w, "\nSERVICE LABELS")
	writeKeys(w, ServiceLabels())

	fmt.Fprintln(w, "\nPORT RESOLUTION, FIRST MATCH WINS")
	for i, p := range PortOrder() {
		fmt.Fprintf(w, "  %d. %s\n", i+1, p)
	}

	fmt.Fprintln(w, "\nSTANDARD COMPOSE KEYS APPLIED")
	writeKeys(w, ComposeKeys())

	fmt.Fprintln(w, "\nEXAMPLE")
	fmt.Fprint(w, indent(Example(), "  "))
}

// SchemaJSON writes the Compose contract as JSON.
func SchemaJSON(w io.Writer) error {
	doc := map[string]any{
		"composeFileNames": []string{
			"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml",
		},
		"projectKeys":   ProjectKeys(),
		"serviceLabels": ServiceLabels(),
		"composeKeys":   ComposeKeys(),
		"portOrder":     PortOrder(),
		"example":       Example(),
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// Example returns a Compose file using every key this tool reads.
func Example() string {
	return `name: shop

x-bay:
  domain: shop.test          # optional; the machine default is used otherwise

services:
  web:
    image: node:26-slim
    ports: ["3000"]          # the container port; no host port is published
    environment:
      DATABASE_URL: postgres://shop-db.container.test:5432/app
    volumes:
      - ./src:/app/src
    command: ["npm", "run", "dev"]
    # served at https://web.shop.test/

  api:
    image: node:26-slim
    expose: ["8080"]
    labels:
      bay.domain: api.shop.test

  db:
    image: postgres:18
    expose: ["5432"]
    environment:
      POSTGRES_PASSWORD: dev
    labels:
      bay.internal: "true"
`
}

// Help writes the detail for one command and reports whether it was found.
func Help(w io.Writer, name string) bool {
	for _, c := range Commands() {
		if c.Name != name {
			continue
		}
		head := "bay " + c.Name
		if c.Args != "" {
			head += " " + c.Args
		}
		fmt.Fprintf(w, "%s\n\n%s\n", head, wrap(c.Summary, 78))
		if c.Effect != "" {
			fmt.Fprintf(w, "\nEffect\n%s\n", indent(wrap(c.Effect, 74), "  "))
		}
		if c.Root {
			fmt.Fprintln(w, "\nThis command can require administrator rights.")
		}
		if c.Example != "" {
			fmt.Fprintf(w, "\nExample\n  %s\n", c.Example)
		}
		return true
	}
	return false
}

// CommandList writes the command table used by the usage text.
func CommandList(w io.Writer) {
	names := make([]string, 0, len(Commands()))
	width := 0
	for _, c := range Commands() {
		name := c.Name
		if c.Args != "" {
			name += " " + c.Args
		}
		if len(name) > width {
			width = len(name)
		}
		names = append(names, name)
	}
	for i, c := range Commands() {
		fmt.Fprintf(w, "  %-*s  %s\n", width, names[i], c.Summary)
	}
}

func writeKeys(w io.Writer, keys []Key) {
	for _, k := range keys {
		fmt.Fprintf(w, "  %s\n", k.Path)
		fmt.Fprintf(w, "      type %s, default %s\n", k.Type, k.Default)
		fmt.Fprintf(w, "%s\n", indent(wrap(k.Description, 68), "      "))
	}
}

func indent(s, prefix string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = prefix + l
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func wrap(s string, width int) string {
	var out, line strings.Builder
	for _, word := range strings.Fields(s) {
		if line.Len() > 0 && line.Len()+1+len(word) > width {
			out.WriteString(line.String())
			out.WriteByte('\n')
			line.Reset()
		}
		if line.Len() > 0 {
			line.WriteByte(' ')
		}
		line.WriteString(word)
	}
	out.WriteString(line.String())
	return out.String()
}
