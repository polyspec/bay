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

	fmt.Fprintln(w, "\nFIRST STEPS")
	fmt.Fprintln(w, "  bay doctor        is this machine set up")
	fmt.Fprintln(w, "  bay status        what is already running here")
	fmt.Fprintln(w, "  bay domain        which domains this machine serves")
	fmt.Fprintln(w, "  cd <your project>")
	fmt.Fprintln(w, "  bay up            start it and serve it over HTTPS")

	fmt.Fprintln(w, "\nWHAT UP DOES")
	fmt.Fprintln(w, "  Reads the Compose file, starts one container per service, issues a")
	fmt.Fprintln(w, "  certificate for each domain, and adds the routes to the machine's")
	fmt.Fprintln(w, "  proxy. It returns after the proxy serves them. Nothing is published")
	fmt.Fprintln(w, "  to a host port. On macOS /etc/hosts is not touched.")

	fmt.Fprintln(w, "\nCOMPOSE FILE")
	fmt.Fprintln(w, "  A project is a Compose file: compose.yaml, compose.yml,")
	fmt.Fprintln(w, "  docker-compose.yaml or docker-compose.yml. Settings live under the")
	fmt.Fprintln(w, "  top-level x-bay mapping, a service's x-bay mapping")
	fmt.Fprintln(w, "  and bay.* service labels. A service is served at the domains")
	fmt.Fprintln(w, "  listed in its x-bay.domains; a service without the list is")
	fmt.Fprintln(w, "  internal.")
	fmt.Fprint(w, "\n", indent(Example(), "    "))
	fmt.Fprintln(w, "\n  Run \"bay schema\" for every key and its default.")

	fmt.Fprintln(w, "\nDOMAINS")
	fmt.Fprintln(w, "  A domain is delegated once for the whole machine. A project uses the")
	fmt.Fprintln(w, "  machine default unless its Compose file names one under")
	fmt.Fprintln(w, "  x-bay.domain.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "    bay domain                 list them")
	fmt.Fprintln(w, "    bay domain add lab.test    delegate another, asks for the password")
	fmt.Fprintln(w, "    bay domain default lab.test")
	fmt.Fprintln(w, "    bay domain remove lab.test")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  A two-label domain such as lab.test also covers names that have no")
	fmt.Fprintln(w, "  route: they receive 404 instead of a certificate warning. A")
	fmt.Fprintln(w, "  single-label domain does not, because clients reject a wildcard whose")
	fmt.Fprintln(w, "  parent is one label.")

	fmt.Fprintln(w, "\nSERVICE TO SERVICE")
	fmt.Fprintln(w, "  Reach another service at <project>-<service>.bay.test:<port>.")
	fmt.Fprintln(w, "  Container addresses change on every start, so use the name.")

	fmt.Fprintln(w, "\nWHAT IT IS FOR")
	for _, d := range Duties() {
		fmt.Fprintf(w, "  - %s\n", d.Duty)
	}
	fmt.Fprintln(w, "\nWHAT IT DOES NOT DO")
	for _, e := range Exclusions() {
		fmt.Fprintf(w, "  - %s Instead: %s\n", e.Excluded, e.Instead)
	}

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

	fmt.Fprintln(w, "\nFLAGS BEFORE THE COMMAND")
	for _, f := range GlobalFlags() {
		fmt.Fprintf(w, "  %s %s\n%s", f.Name, f.Value, indent(wrap(f.Description, 70), "      "))
	}

	fmt.Fprintln(w, "\nEXIT STATUS")
	for _, e := range ExitCodes() {
		fmt.Fprintf(w, "  %d  %s\n", e.Code, strings.TrimPrefix(indent(wrap(e.Meaning, 72), "     "), "     "))
	}

	fmt.Fprintln(w, "\nMACHINE STATE")
	for _, s := range StateFiles() {
		if s.Root {
			fmt.Fprintf(w, "  %-48s  requires administrator rights\n", s.Path)
			continue
		}
		fmt.Fprintf(w, "  %s\n", s.Path)
	}
	fmt.Fprintln(w, "\nMORE")
	fmt.Fprintln(w, "  bay schema        every Compose key and its default")
	fmt.Fprintln(w, "  bay help <cmd>    what one command changes")
	fmt.Fprintln(w, "  bay status --json the current state, for another program")
	fmt.Fprintln(w, "  bay skill         the same contract as an agent skill")
	fmt.Fprintln(w, "  bay mcp           the commands as MCP tools, for an agent")
}

// BriefJSON writes the same contract as JSON.
func BriefJSON(w io.Writer) error {
	doc := map[string]any{
		"schemaVersion": SchemaVersion,
		"purpose":       Purpose,
		"commands":      Commands(),
		"exitCodes":     ExitCodes(),
		"globalFlags":   GlobalFlags(),
		"projectKeys":   ProjectKeys(),
		"serviceKeys":   ServiceKeys(),
		"serviceLabels": ServiceLabels(),
		"composeKeys":   ComposeKeys(),
		"portOrder":     PortOrder(),
		"duties":        Duties(),
		"exclusions":    Exclusions(),
		"invariants":    Invariants(),
		"diagnostics":   Diagnostics(),
		"stateFiles":    StateFiles(),
		"composeFileNames": []string{
			"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml",
		},
		"serviceAddress": "<project>-<service>.bay.test:<port>",
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

	fmt.Fprintln(w, "\nSERVICE KEYS")
	writeKeys(w, ServiceKeys())

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
		"serviceKeys":   ServiceKeys(),
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
      DATABASE_URL: postgres://shop-db.bay.test:5432/app
    volumes:
      - ./src:/app/src
    command: ["npm", "run", "dev"]
    x-bay:
      domains: [web.shop.test]
    # served at https://web.shop.test/

  api:
    image: node:26-slim
    expose: ["8080"]
    x-bay:
      domains:
        - api.shop.test
        - admin.shop.test
    # served at https://api.shop.test/ and https://admin.shop.test/

  db:
    image: postgres:18
    expose: ["5432"]
    environment:
      POSTGRES_PASSWORD: dev
    # no domains: internal, reached at shop-db.bay.test:5432
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
			fmt.Fprintf(w, "\nEffect\n%s", indent(wrap(c.Effect, 74), "  "))
		}
		fmt.Fprintf(w, "\nAccess\n  %s\n", accessText(c.Access))
		if c.Asks != "" {
			fmt.Fprintf(w, "\nAsks a person for\n%s", indent(wrap(c.Asks, 74), "  "))
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

// accessText says what an access level lets a caller do without asking.
func accessText(a Access) string {
	switch a {
	case Reads:
		return "read: changes nothing"
	case Changes:
		return "change: creates, starts or rewrites; running it again changes nothing more"
	case Removes:
		return "remove: can remove containers, routes, approvals or setup"
	}
	return string(a)
}
