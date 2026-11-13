package contract

import (
	"fmt"
	"io"
	"strings"
)

// An agent learns this tool three ways, all rendered here from the same
// declarations as the command help: the skill an agent loads when a task
// matches its description, the MCP server's instructions and tools, and the
// generated section of the README.

// SkillName is the skill's name and the directory it is installed in.
const SkillName = "bay"

// SkillDescription is what an agent matches a task against to load the skill.
const SkillDescription = "Run Compose projects as HTTPS local domains (*.test) on Apple container " +
	"or Docker with bay, reach the domains of peer machines, and check or repair the " +
	"machine setup. Use when starting, stopping, inspecting or debugging a Compose project it runs, " +
	"a *.test name, a certificate or trust error, a peer machine, or the tool itself."

// Skill writes SKILL.md.
func Skill(w io.Writer) {
	fmt.Fprintln(w, "---")
	fmt.Fprintf(w, "name: %s\n", SkillName)
	fmt.Fprintf(w, "description: %s\n", SkillDescription)
	fmt.Fprintln(w, "metadata:")
	fmt.Fprintf(w, "  schemaVersion: \"%d\"\n", SchemaVersion)
	fmt.Fprintln(w, "  generated: \"bay skill print; do not edit, run bay skill install\"")
	fmt.Fprintln(w, "---")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "# bay")
	fmt.Fprintln(w)
	fmt.Fprintln(w, Purpose)
	fmt.Fprintln(w)
	scopeMarkdown(w, "##")
	fmt.Fprintln(w)
	agentRules(w)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Commands")
	fmt.Fprintln(w)
	commandTable(w)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Flags before the command")
	fmt.Fprintln(w)
	for _, f := range GlobalFlags() {
		fmt.Fprintf(w, "- `%s %s` %s\n", f.Name, f.Value, f.Description)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Exit status")
	fmt.Fprintln(w)
	for _, e := range ExitCodes() {
		fmt.Fprintf(w, "- `%d` %s\n", e.Code, e.Meaning)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Compose file")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "A project is a Compose file. A service is served at the domains listed in its")
	fmt.Fprintln(w, "`x-bay.domains`; a service without the list is internal and is reached")
	fmt.Fprintln(w, "by other services at `<project>-<service>.bay.test:<port>`. Run")
	fmt.Fprintln(w, "`bay schema` for every key and its default.")
	fmt.Fprintf(w, "\n```yaml\n%s```\n", Example())
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Rules the tool holds to")
	fmt.Fprintln(w)
	for _, i := range Invariants() {
		fmt.Fprintf(w, "- %s %s\n", i.Rule, i.Reason)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Symptoms")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| Symptom | Cause | Run |")
	fmt.Fprintln(w, "| --- | --- | --- |")
	for _, d := range Diagnostics() {
		fmt.Fprintf(w, "| %s | %s | `%s` |\n", d.Symptom, d.Cause, d.Command)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Files the tool owns")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Do not edit these; the commands write them and an edit is replaced or breaks the check.")
	fmt.Fprintln(w)
	for _, s := range StateFiles() {
		fmt.Fprintf(w, "- `%s` %s\n", s.Path, s.Content)
	}
}

// agentRules writes how an agent uses the tool. They follow from the access
// and the questions the commands declare.
func agentRules(w io.Writer) {
	fmt.Fprintln(w, "## How to use it")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "1. Read before changing: `bay check` (machine setup, exit 1 when a part is")
	fmt.Fprintln(w, "   missing), `bay status --json` (what runs and at which names),")
	fmt.Fprintln(w, "   `bay brief --json` (this contract, with `schemaVersion`).")
	fmt.Fprintln(w, "2. Run project commands in the directory of the project's Compose file, or pass")
	fmt.Fprintln(w, "   `-f path/compose.yaml`.")
	fmt.Fprintln(w, "3. Machine setup is only `bay install`: run `bay install -dry-run`")
	fmt.Fprintln(w, "   first, then `bay install`, then `bay check`. Never set up a")
	fmt.Fprintln(w, "   resolver, hosts entry, trust store, launchd job, systemd unit or firewall by hand.")
	fmt.Fprintln(w, "4. Commands with access `read` are safe to run at any time. Commands with access")
	fmt.Fprintln(w, "   `remove` can remove what a person relies on; run them when the task asks for it.")
	fmt.Fprintln(w, "5. A command whose \"Asks\" is set puts a question in front of a person. With no")
	fmt.Fprintln(w, "   one at the keyboard, ask the user to run it, for example `! bay peer add <address>`.")
	fmt.Fprintln(w, "6. A service reaches another project's or a peer's name over HTTPS with no extra")
	fmt.Fprintln(w, "   setup: its container trusts this machine's authority. Do not add `-k`,")
	fmt.Fprintln(w, "   `verify=False` or copied certificates.")
	fmt.Fprintln(w, "7. Report what a command printed and its exit status; when it fails, run the")
	fmt.Fprintln(w, "   command standard error names before trying anything else.")
	fmt.Fprintln(w, "8. Run a test suite, a build or other work that ends as a job: a service with")
	fmt.Fprintln(w, "   `x-bay.job: true`, queued with `bay job add -key <commit> <service>` and read")
	fmt.Fprintln(w, "   with `bay job wait <id>`. The machine runs one job at a time, so heavy runs")
	fmt.Fprintln(w, "   wait their turn instead of starving each other. On another machine, queue")
	fmt.Fprintln(w, "   it there over ssh and wait with `bay job wait -peer <name> <id>`, which")
	fmt.Fprintln(w, "   survives a dropped link; queueing again with the same `-key` never runs twice.")
}

// commandTable writes one row per command: the command line, what it does, its
// access and what it asks a person for.
func commandTable(w io.Writer) {
	fmt.Fprintln(w, "| Command | Summary | Access | Asks a person for |")
	fmt.Fprintln(w, "| --- | --- | --- | --- |")
	for _, c := range Commands() {
		name := "bay " + c.Name
		if c.Args != "" {
			name += " " + c.Args
		}
		asks := "nothing"
		if c.Asks != "" {
			asks = c.Asks
		}
		fmt.Fprintf(w, "| `%s` | %s | %s | %s |\n", cell(name), cell(c.Summary), c.Access, cell(asks))
	}
}

// MarkdownAgents writes the README's section on agents.
func MarkdownAgents(w io.Writer) {
	scopeMarkdown(w, "###")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "```sh")
	fmt.Fprintln(w, "bay install                               # also writes the skill below")
	fmt.Fprintln(w, "bay skill install                         # ~/.claude/skills and ~/.agents/skills")
	fmt.Fprintln(w, "bay mcp install                                    # registers it in Claude Code; install does too")
	fmt.Fprintln(w, "bay brief --json                          # the contract, schemaVersion "+fmt.Sprint(SchemaVersion))
	fmt.Fprintln(w, "```")
	fmt.Fprintln(w)
	commandTable(w)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| Exit status | Meaning |")
	fmt.Fprintln(w, "| --- | --- |")
	for _, e := range ExitCodes() {
		fmt.Fprintf(w, "| `%d` | %s |\n", e.Code, e.Meaning)
	}
}

// MCPInstructions is what the MCP server tells a client about itself.
func MCPInstructions() string {
	var b strings.Builder
	b.WriteString(Purpose + "\n\n")
	b.WriteString("Every tool runs one bay command. Pass `dir`, the directory of the " +
		"project's Compose file, for project commands, and `args`, the arguments after the " +
		"command name. A tool's result is the command's output and its exit status.\n\n")
	b.WriteString("Read first: check, status (args [\"--json\"]), brief. Machine setup is only " +
		"install: run it with args [\"-dry-run\"] first. A command that asks a person a question " +
		"gets no answer here; ask the user to run it in a terminal. Work that ends, such as a test suite, is a job: " +
		"queue it with job args [\"add\", \"-key\", \"<commit>\", \"<service>\"], which returns its ID at once, then call job with " +
		"[\"wait\", \"<id>\"] (or [\"wait\", \"-peer\", \"<name>\", \"<id>\"] for a peer); a wait states its bound and a dropped link is read again. The resources " +
		"bay://brief, bay://schema and bay://status hold the contract " +
		"and the current state as JSON.\n")
	return b.String()
}

// cell escapes the pipes a table cell's text holds, which would otherwise end
// the cell, such as those between a command's alternatives.
func cell(s string) string { return strings.ReplaceAll(s, "|", "\\|") }
