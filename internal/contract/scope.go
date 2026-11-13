package contract

import (
	"fmt"
	"io"
)

// The tool's task, and what it leaves to others. A request is weighed against
// these before it is built: what serves the task is the tool's work, and what
// does not stays out however often it is asked for, because each exclusion
// keeps a promise the task depends on.

// Duty is a part of the task.
type Duty struct {
	Duty   string `json:"duty"`
	Detail string `json:"detail"`
}

// Duties are what the tool is for.
func Duties() []Duty {
	return []Duty{
		{"Run a Compose project as its file says, on Apple container or Docker.",
			"A key the Compose specification defines behaves as it does there, or is rejected by name; nothing is silently ignored."},
		{"Give every service a name, a certificate and trust.",
			"<project>-<service>.bay.test, known before up from the file alone; a certificate for that name from this machine's authority; containers that trust the authority."},
		{"Serve the domains a project declares over HTTPS.",
			"One proxy per machine, routes from container labels, certificates issued and renewed by the tool."},
		{"Reach the names of approved peer machines.",
			"Over the mutually authenticated peer link, and only to machines approved by fingerprint."},
		{"Be a dependable step for a person, a script or an agent.",
			"Exit status that says whether the request was done, JSON output where a program reads the result, nothing left behind by an interrupted command, and the same result when run twice."},
		{"Keep a Linux machine reachable while projects use it.",
			"The containers bay runs share a slice that slows them past 75% of memory and holds them under 85%, and the kernel keeps a reserve for the network driver, so a loaded machine is slow but answers."},
		{"Apply the machine setup only on request.",
			"install applies what is missing; every other command reads the setup and names install when a part is missing."},
	}
}

// Exclusion is something the tool does not do, and why.
type Exclusion struct {
	Excluded string `json:"excluded"`
	Reason   string `json:"reason"`
	Instead  string `json:"instead"`
}

// Exclusions are what the tool leaves to others.
func Exclusions() []Exclusion {
	return []Exclusion{
		{"Running or stopping a project on another machine.",
			"A project's files, volumes, state and setup are on the machine that runs it; a remote up would act on paths and setup it cannot see.",
			"Run the command on the machine that holds the project, for example over ssh: ssh host bay -f /path/compose.yaml up."},
		{"Publishing a service port or a Unix socket on the host.",
			"Every service is reached by name, so two projects never compete for a host port and a reachable service is always a named, certified one.",
			"Connect to <project>-<service>.bay.test:<port>; a socket in a bind-mounted directory is plain Compose and works where the engine shares the host's kernel."},
		{"Waiting without a bound.",
			"A wait with no limit hides a service that never starts; every wait states its bound before it begins.",
			"Give a slow service a longer healthcheck start_period or interval."},
		{"Deciding a caller's policy: retries, time limits, which images to fetch.",
			"These belong to the caller's rules; the tool states what it does and offers the switches.",
			"bay pull, up -no-pull, healthcheck keys, exit status."},
		{"Changing the machine setup from a project command.",
			"A setup change asks for administrator rights and affects every project; it happens when a person asks for it.",
			"bay install -dry-run, then bay install."},
	}
}

// scopeMarkdown writes the task and the exclusions as Markdown sections at the
// given heading level.
func scopeMarkdown(w io.Writer, heading string) {
	fmt.Fprintf(w, "%s What it is for\n\n", heading)
	for _, d := range Duties() {
		fmt.Fprintf(w, "- **%s** %s\n", d.Duty, d.Detail)
	}
	fmt.Fprintf(w, "\n%s What it does not do\n\n", heading)
	fmt.Fprintln(w, "| Not done | Why | Instead |")
	fmt.Fprintln(w, "| --- | --- | --- |")
	for _, e := range Exclusions() {
		fmt.Fprintf(w, "| %s | %s | %s |\n", e.Excluded, e.Reason, e.Instead)
	}
}
