// Package contract declares the command line and Compose file contract. The
// command help, the brief and schema commands, and the generated sections of
// the specification documents all render from these declarations, so they
// state the same behavior.
package contract

// Purpose states what this tool does.
const Purpose = "bay runs Compose projects on Apple `container` and " +
	"serves each service over HTTPS at a local domain name. No host port is published."

// Command declares one command line command.
type Command struct {
	Name string
	// Args is the argument summary printed after the command name.
	Args string
	// Summary is one line.
	Summary string
	// Effect states what the command changes. It is empty when the command
	// changes nothing.
	Effect string
	// Root reports whether the command can require administrator rights.
	Root bool
	// Example is a command line.
	Example string
}

// Commands returns every command in display order.
func Commands() []Command {
	return []Command{
		{
			Name: "up", Args: "",
			Summary: "Start this project's services and register their routes.",
			Effect: "Reuses unchanged owned containers and proven completed initializers; replaces changed configuration or local image digests. " +
				"Starts dependencies in order, checks declared health, issues certificates and updates the proxy. Delegates new project domains.",
			Root:    true,
			Example: "bay up",
		},
		{
			Name: "down", Args: "",
			Summary: "Remove this project's services and withdraw its routes.",
			Effect:  "Removes the project's containers. Removes the proxy when no route remains.",
			Example: "bay down",
		},
		{
			Name: "start", Args: "[service...]",
			Summary: "Start services. With no name, starts every service in the project.",
			Effect:  "Reconciles selected services and their dependencies, reuses unchanged containers, checks startup conditions, then updates the proxy.",
			Example: "bay start web",
		},
		{
			Name: "stop", Args: "[service...]",
			Summary: "Stop services. The containers remain and can be started again.",
			Effect:  "Stops containers and withdraws their routes.",
			Example: "bay stop db",
		},
		{
			Name: "restart", Args: "[service...]",
			Summary: "Stop then start services.",
			Effect:  "Recreates selected owned containers, including explicitly retried initializers, after preparing their dependencies; updates the proxy.",
			Example: "bay restart web",
		},
		{
			Name: "logs", Args: "[-f] [-n N] <service>",
			Summary: "Print one service's output.",
			Example: "bay logs -f web",
		},
		{
			Name: "domain", Args: "[add|remove|default] [name]",
			Summary: "List the machine's domains, or change them.",
			Effect: "add writes an /etc/resolver entry for the domain. remove deletes it. " +
				"default changes the domain projects use when their Compose file names none.",
			Root:    true,
			Example: "bay domain add lab.test",
		},
		{
			Name: "status", Args: "[--json]",
			Summary: "Print the proxy, the DNS server and every registered project.",
			Effect:  "Reads state without registering the selected Compose project, creating certificates, or changing machine setup. Missing or unreadable public certificates are reported; private keys are not read.",
			Example: "bay status --json",
		},
		{
			Name: "doctor", Args: "",
			Summary: "Report what machine setup is missing. Changes nothing.",
			Effect:  "Reads public certificate and setup state, including the selected project's domains without saving them. Creates no files and reads no private keys.",
			Example: "bay doctor",
		},
		{
			Name: "install", Args: "",
			Summary: "Apply the machine setup now instead of during the next up.",
			Effect: "Writes /etc/resolver entries, adds the certificate authority to " +
				"the user's trust settings, and registers the DNS launchd agent.",
			Root:    true,
			Example: "bay install",
		},
		{
			Name: "uninstall", Args: "",
			Summary: "Remove the DNS agent and the resolver entries.",
			Effect: "Removes /etc/resolver entries and the launchd agent. " +
				"The certificate authority stays in the trust settings.",
			Root:    true,
			Example: "bay uninstall",
		},
		{
			Name: "brief", Args: "[--json]",
			Summary: "Print the full usage contract on one screen.",
			Example: "bay brief",
		},
		{
			Name: "schema", Args: "[--json]",
			Summary: "Print the Compose extension contract.",
			Example: "bay schema",
		},
		{
			Name: "help", Args: "[command]",
			Summary: "Print help for one command, or the command list.",
			Example: "bay help up",
		},
	}
}

// Key declares one Compose file key this tool reads.
type Key struct {
	Path        string
	Type        string
	Default     string
	Description string
}

// ProjectKeys are the keys under the top-level `x-bay` mapping.
func ProjectKeys() []Key {
	return []Key{
		{"x-bay.domain", "string", "the machine default domain",
			"Local domain this project's services use. Naming it here pins it to the project."},
		{"x-bay.extra_domains", "list of string", "empty",
			"Further local domains this project's services may claim."},
		{"x-bay.network", "string", "default",
			"Container network the project's services and the proxy share."},
		{"name", "string", "the Compose file's directory name",
			"Project name. Prefixes every container name."},
	}
}

// ServiceLabels are the labels this tool reads from a Compose service.
func ServiceLabels() []Key {
	return []Key{
		{"bay.domain", "string", "<service>.<project domain>",
			"Domain the proxy routes to this service."},
		{"bay.port", "integer", "see the port order below",
			"Port the service listens on inside the container."},
		{"bay.internal", "boolean", "false",
			"Marks a service with no domain. It runs and other services reach it by name, " +
				"but the proxy does not route to it and issues it no certificate."},
		{"bay.tls", "boolean", "false",
			"Marks a backend that already serves HTTPS on its port."},
	}
}

// InternalLabels are labels this tool sets on the containers it creates. They
// are not read from a Compose file; callers do not set them.
func InternalLabels() []Key {
	return []Key{
		{"bay.role", "string", "none", "Marks a container as a service or the proxy."},
		{"bay.group", "string", "none", "Project the container belongs to."},
		{"bay.service", "string", "none", "Service name within the project."},
		{"bay.scheme", "string", "http", "Scheme the proxy uses to reach the service."},
		{"bay.config", "string", "none", "Digest of the resolved service configuration and local image."},
		{"bay.volume", "string", "none", "Compose declaration key of a managed named volume."},
	}
}

// PortOrder lists where the container port is read from, first match wins.
func PortOrder() []string {
	return []string{
		"the bay.port label",
		"the first entry of `expose`",
		"the container side of the first entry of `ports`",
		"80",
	}
}

// ComposeKeys lists the standard Compose keys this tool applies.
func ComposeKeys() []Key {
	return []Key{
		{"image", "string", "required", "Image to run."},
		{"command", "string or list", "the image's command", "Process arguments."},
		{"entrypoint", "string or list", "the image's entrypoint", "Placed before command."},
		{"environment", "mapping or list", "empty", "Environment variables."},
		{"volumes", "list of string", "empty", "Compose-relative bind mounts or declared named volumes, `source:target[:ro]`. Top-level volumes supports name, external, driver: local and driver_opts.size; external volumes allow only name."},
		{"user", "string", "image default", "Process user, name or uid[:gid]. Values may use project .env and environment interpolation."},
		{"read_only", "boolean", "false", "Mount the container root filesystem read-only."},
		{"cap_drop", "list of string", "empty", "Linux capabilities to drop, including ALL."},
		{"depends_on", "list or mapping", "empty", "Startup dependencies: service_started, service_healthy or service_completed_successfully. Unsupported options and cycles are rejected."},
		{"healthcheck", "mapping", "empty", "Startup CMD/CMD-SHELL test; interval, timeout, retries, start_period and disable. No continuous background monitoring."},
		{"container_name", "string", "<project>-<service>", "Container name."},
		{"networks", "list or mapping", "the project network", "First entry is used."},
		{"mem_limit", "string", "runtime default", "Memory limit."},
		{"cpus", "string", "runtime default", "CPU count."},
	}
}

// Invariant is a rule callers can rely on.
type Invariant struct {
	Rule   string
	Reason string
}

// Invariants are the rules this tool holds to.
func Invariants() []Invariant {
	return []Invariant{
		{"One proxy container per machine, named bay-edge.",
			"Projects share it. Its configuration is rebuilt from every running project."},
		{"Routes are read from container labels, not from Compose files.",
			"Removing a project's containers removes its routes without editing a file."},
		{"Domains are machine state, delegated once in /etc/resolver.",
			"A project inherits the machine default unless its Compose file pins one."},
		{"Certificates are issued and reissued automatically.",
			"One per routed domain, plus a default certificate for unrouted names."},
		{"Container addresses change on every start.",
			"The proxy names backends and resolves them per request, so addresses are never pinned."},
		{"up, down, start, stop and restart return after the proxy serves the new configuration.",
			"Reloading nginx is asynchronous, so the commands poll the proxy's health endpoint."},
		{"Only /etc/resolver writes require administrator rights.",
			"Trusting the certificate authority uses the user's trust settings."},
	}
}

// Diagnostic maps a symptom to its cause and a command that reports more.
type Diagnostic struct {
	Symptom string
	Cause   string
	Command string
}

// Diagnostics lists the common failures and their diagnostic commands.
func Diagnostics() []Diagnostic {
	return []Diagnostic{
		{"A name does not resolve.", "The DNS agent is not registered.", "bay doctor"},
		{"502 from the proxy.", "The container runs but does not listen on the resolved port.",
			"bay logs <service>"},
		{"404 from the proxy.", "No route claims that name. The service is stopped, internal, or the domain differs.",
			"bay status"},
		{"Certificate warning.", "The certificate authority is not trusted, or the name has no route.",
			"bay install"},
		{"A command asks for a password.", "A new domain requires an /etc/resolver entry.",
			"bay doctor"},
	}
}

// StateFile is one path this tool creates outside the repository.
type StateFile struct {
	Path    string
	Content string
	Root    bool
}

// StateFiles are the paths this tool writes.
func StateFiles() []StateFile {
	return []StateFile{
		{"/etc/resolver/<domain>", "Delegates the domain to the DNS server.", true},
		{"~/Library/LaunchAgents/dev.bay.dns.plist", "Runs the DNS server.", false},
		{"~/.bay/ca.crt, ca.key", "The certificate authority.", false},
		{"~/.bay/certs/", "Issued certificates.", false},
		{"~/.bay/conf.d/stack.conf", "The generated proxy configuration.", false},
		{"~/.bay/machine.json", "The machine's domains.", false},
		{"~/.bay/groups.json", "Registered projects.", false},
	}
}
