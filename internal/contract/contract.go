// Package contract declares the command line and Compose file contract. The
// command help, the brief and schema commands, and the generated sections of
// the specification documents all render from these declarations, so they
// state the same behavior.
package contract

// Purpose states what this tool does.
const Purpose = "bay runs Compose projects on Apple `container` and on Docker, " +
	"serves each service over HTTPS at a local domain name, and reaches the domains of " +
	"approved peer machines. No service port is published."

// SchemaVersion is the version of this contract's machine-readable form:
// brief --json, schema --json, the agent skill and the MCP server. It changes
// when a field is removed or changes meaning.
const SchemaVersion = 1

// Access says what a command can do to the machine, so a caller, a person or an
// agent, knows which commands it may run without asking.
type Access string

const (
	// Reads changes nothing.
	Reads Access = "read"
	// Changes creates, starts or rewrites things. Running it again with the
	// same input changes nothing more.
	Changes Access = "change"
	// Removes can remove containers, routes, approvals or machine setup.
	Removes Access = "remove"
)

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
	// Access is what the command can do to the machine.
	Access Access
	// Asks is what the command can put in front of a person, empty when it
	// never asks. A caller with no one at the keyboard cannot answer it.
	Asks string
	// Terminal marks a command that is not offered as an agent tool: it serves
	// the tools itself, or prints what a resource already holds.
	Terminal bool
}

const (
	asksPassword = "the administrator password, in a system dialog or on the terminal, when a step needs root"
	asksApproval = "approval of the other machine's fingerprint, read from standard input; with no one to answer, nothing is approved"
)

// Flag declares one flag given before the command.
type Flag struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	Description string `json:"description"`
}

// GlobalFlags are the flags every command takes before its name.
func GlobalFlags() []Flag {
	return []Flag{
		{"-f", "path", "The Compose file, or a directory to find one in. Default: the working directory."},
		{"-p", "name", "The project name, instead of the Compose file's name or directory. Two runs of one file under two names " +
			"do not overlap: their containers, default volume names and registrations are separate, and down -p removes only its own."},
		{"-state", "dir", "The state directory. Default: ~/.bay."},
	}
}

// ExitCode declares what one exit status means.
type ExitCode struct {
	Code    int
	Meaning string
}

// ExitCodes are the exit statuses of every command.
func ExitCodes() []ExitCode {
	return []ExitCode{
		{0, "The command did what it states. For check, every part of the setup is in place."},
		{1, "The command failed, or check found a part missing. Standard error says why and names the command that reports more."},
		{2, "The command line names no command or an unknown one."},
	}
}

// Commands returns every command in display order.
func Commands() []Command {
	return []Command{
		{
			Name: "up", Args: "[-no-pull] [--json]",
			Summary: "Start this project's services and register their routes.",
			Effect: "Reuses unchanged owned containers and proven completed initializers; replaces changed configuration or local image digests. " +
				"Starts dependencies in order, checks declared health, issues certificates and updates the proxy. Delegates new project domains. " +
				"Removes the containers of services the Compose file no longer declares, stopping a running one first, and withdraws their routes. " +
				"-no-pull fails before creating anything when the local store lacks an image, instead of pulling it. " +
				"--json prints each service's name, host name, address, port, URLs and certificate paths on standard output, and progress on standard error. " +
				"On SIGINT or SIGTERM it ends the engine commands that run, removes the containers this up created and leaves the ones that were there before, " +
				"withdraws the registration of a project that had no container before, keeps volumes, and exits with 128 plus the signal number.",
			Root:    true,
			Example: "bay up",
			Access:  Changes,
			Asks:    asksPassword,
		},
		{
			Name: "down", Args: "",
			Summary: "Remove this project's services and withdraw its routes.",
			Effect:  "Removes the project's containers. Removes the proxy when no route remains.",
			Example: "bay down",
			Access:  Removes,
		},
		{
			Name: "start", Args: "[service...]",
			Summary: "Start services. With no name, starts every service in the project.",
			Effect:  "Reconciles selected services and their dependencies, reuses unchanged containers, checks startup conditions, then updates the proxy.",
			Example: "bay start web",
			Access:  Changes,
		},
		{
			Name: "stop", Args: "[service...]",
			Summary: "Stop services. The containers remain and can be started again.",
			Effect:  "Stops containers and withdraws their routes.",
			Example: "bay stop db",
			Access:  Changes,
		},
		{
			Name: "restart", Args: "[service...]",
			Summary: "Stop then start services.",
			Effect:  "Recreates selected owned containers, including explicitly retried initializers, after preparing their dependencies; updates the proxy.",
			Example: "bay restart web",
			Access:  Changes,
		},
		{
			Name: "logs", Args: "[-f] [-n N] <service>",
			Summary: "Print one service's output.",
			Example: "bay logs -f web",
			Access:  Reads,
		},
		{
			Name: "plan", Args: "[--json]",
			Summary: "Print each service's name, port, domains and certificate paths as up will create them.",
			Example: "bay plan --json",
			Access:  Reads,
		},
		{
			Name: "job", Args: "add [-wait] [-key k] [-timeout d] <service> | list [-peer name] | show [-peer name] <id> | logs [-f] [-peer name] <id> | wait [-peer name] [-timeout d] <id> | cancel <id> | schedule | unschedule",
			Summary: "Queue a project's job services and read how they ran. The machine runs one job at a time, oldest first.",
			Effect: "add queues a run of a service declared with x-bay.job and prints its ID; the agent runs it when the jobs queued before it have ended: " +
				"it brings the project's other services up as up does, runs the job's container to completion with its output in the job's log, " +
				"removes the container, and takes the project down again when it was not running before. " +
				"-key names the run, such as a commit: a job of the service with that key is returned instead of a second one, so a caller cut off before it read the ID queues again safely. " +
				"-wait prints the output as it comes and exits 0 when the job succeeded and 1 when it did not, after at most -timeout (2h). " +
				"wait does the same without the output; with -peer it reads an approved peer's job over the link and, when the link drops, reads again every 5s until the bound, " +
				"because the job runs on there and keeps its result. cancel removes a queued job, or stops a running one with its stop signal and grace period. " +
				"schedule makes the agent queue the project's jobs at their x-bay.schedule; unschedule stops it. list reads the latest 50 jobs and the schedules, " +
				"of this machine or with -peer of an approved peer; the latest 10 carry the end of their output, which logs -peer prints. " +
				"A job's limit is its x-bay.timeout (1h). A job runs in a process of its own: a restart or an update of the agent leaves it running and the next agent takes it over. " +
				"Jobs and their output are kept in <state>/jobs, the latest 200. On macOS the agent shows a notification when a job here or on an approved peer ends, " +
				"and one that ended while the peer was out of reach when the link is back. " +
				"A job on another machine is queued there: ssh host bay -f /path/compose.yaml job add -key $COMMIT test, then bay job wait -peer host <id>.",
			Example: "bay job add -wait test",
			Access:  Changes,
		},
		{
			Name: "exec", Args: "[-u user] [-w dir] [-e KEY=VALUE] <service> <command...>",
			Summary: "Run a command in a service's running container and exit with its status.",
			Effect: "Whatever the command does inside the container. Standard input, output and error are the caller's, " +
				"with a terminal when standard input is one. The exit status is the command's own; a status of 1 with " +
				"a line starting with bay: on standard error is bay's own failure.",
			Example: "bay exec db psql -U postgres -c 'select 1'",
			Access:  Changes,
		},
		{
			Name: "pull", Args: "[service...]",
			Summary: "Fetch the images of this project's services from their registries.",
			Effect:  "Pulls each selected service's image, or every service's with no name, whether or not the local store holds it. Creates, starts and changes no container.",
			Example: "bay pull",
			Access:  Changes,
		},
		{
			Name: "images", Args: "[--json] [service...]",
			Summary: "Report whether the local store holds each service's image. Fetches nothing.",
			Effect:  "Prints present or missing per service and exits 1 when any image is missing.",
			Example: "bay images --json",
			Access:  Reads,
		},
		{
			Name: "domain", Args: "[add|remove|default] [name]",
			Summary: "List the machine's domains, or change them.",
			Effect: "add writes an /etc/resolver entry for the domain. remove deletes it. " +
				"default changes the domain projects use when their Compose file names none.",
			Root:    true,
			Example: "bay domain add lab.test",
			Access:  Removes,
			Asks:    asksPassword,
		},
		{
			Name: "status", Args: "[--json]",
			Summary: "Print the proxy, the DNS server and every registered project.",
			Effect:  "Reads state without registering the selected Compose project, creating certificates, or changing machine setup. Missing or unreadable public certificates are reported; private keys are not read.",
			Example: "bay status --json",
			Access:  Reads,
		},
		{
			Name: "doctor", Args: "",
			Summary: "Report what machine setup is missing. Changes nothing.",
			Effect:  "Reads public certificate and setup state, including the selected project's domains without saving them. Creates no files and reads no private keys.",
			Example: "bay doctor",
			Access:  Reads,
		},
		{
			Name: "check", Args: "[-scope all|run]",
			Summary: "Report each part of the machine setup and fail when any is missing. Changes nothing.",
			Effect: "Prints one line per part: ok or missing, and for a missing part what " +
				"\"install\" would do about it. Exits non-zero when any part is missing, " +
				"so a script can call it before relying on the machine. -scope run leaves out the parts that only tell agents about the tool " +
				"(the agent skill and the MCP registration), so a test run can require exactly what running a project needs.",
			Example: "bay check",
			Access:  Reads,
		},
		{
			Name: "peer", Args: "[open|close|find|add|remove] [address|name|fingerprint]",
			Summary: "List the machines whose domains this one reaches, or change them.",
			Effect: "With no argument it lists them and says whether the link is open. " +
				"\"open\" answers the link and announces this machine to the network, " +
				"which another machine reads to approve this one; \"close\" stops both. " +
				"\"find\" lists the machines announcing themselves. \"add\" takes an " +
				"address, or the name of a machine that announced itself, reads what it " +
				"says about itself, checks that it holds the authority it names, prints " +
				"the fingerprint and asks before approving. \"remove\" withdraws an " +
				"approval. Every change rewrites the proxy configuration. Nothing is " +
				"added to the keychain.",
			Example: "bay peer add 192.168.0.42:8443",
			Access:  Removes,
			Asks:    asksApproval,
		},
		{
			Name: "sync", Args: "",
			Summary: "Rewrite the proxy configuration from the containers that are running.",
			Effect: "Writes the server blocks and issues any certificate a route needs, " +
				"then reloads the proxy and waits until it serves the new configuration. " +
				"Starts, stops and changes no container.",
			Example: "bay sync",
			Access:  Changes,
		},
		{
			Name: "cert", Args: "client <name> [-out dir] [--json]",
			Summary: "Write a client certificate the machine's authority issues, for mutual TLS from outside the containers.",
			Effect: "Issues a certificate for name, usable as a client, kept in the state directory and reused while more than 30 days remain, " +
				"and writes cert.pem, key.pem (mode 0600) and ca.pem to the directory -out names, the working directory by default.",
			Example: "bay cert client tests -out ./var/tls",
			Access:  Changes,
		},
		{
			Name: "install", Args: "[-dry-run] [-only-from host,...|none]",
			Summary: "Apply the machine setup that is missing. Running it again changes nothing.",
			Effect: "On macOS: writes /etc/resolver entries, adds the certificate authority " +
				"to the user's trust settings, registers the DNS launchd agent and the peer " +
				"relay launchd daemon. On another system: creates the certificate authority " +
				"and, when -only-from names hosts, an nftables firewall that accepts new " +
				"connections on the default route's interface only from them; the hosts are " +
				"recorded, so the next install keeps them, and \"none\" removes the firewall. " +
				"Each part already in place is left alone. -dry-run prints what would change " +
				"and changes nothing. Registers no project.",
			Root:    true,
			Example: "bay install -dry-run",
			Access:  Changes,
			Asks:    asksPassword,
		},
		{
			Name: "uninstall", Args: "",
			Summary: "Remove the machine setup install wrote.",
			Effect: "On macOS removes /etc/resolver entries, the launchd agent and the relay " +
				"daemon; the certificate authority stays in the trust settings. On another " +
				"system removes the firewall.",
			Root:    true,
			Example: "bay uninstall",
			Access:  Removes,
			Asks:    asksPassword,
		},
		{
			Name: "brief", Args: "[--json]",
			Summary: "Print the full usage contract on one screen.",
			Example: "bay brief",
			Access:  Reads,
		},
		{
			Name: "schema", Args: "[--json]",
			Summary: "Print the Compose extension contract.",
			Example: "bay schema",
			Access:  Reads,
		},
		{
			Name: "skill", Args: "[print|install]",
			Summary: "Print the agent skill, or write it where agents read skills.",
			Effect: "print writes SKILL.md to standard output. install writes it to " +
				"~/.claude/skills/bay/ and ~/.agents/skills/bay/, for each of " +
				"~/.claude and ~/.agents that exists, only when the content differs. " +
				"\"bay install\", \"bay up\", \"make install\" and the MCP server at its start do the same, " +
				"and check reports a skill that is missing or out of date.",
			Example:  "bay skill install",
			Access:   Changes,
			Terminal: true,
		},
		{
			Name: "mcp", Args: "[install]",
			Summary: "Serve the commands to an agent as an MCP server on standard input and output, or register it.",
			Effect: "Offers each command that is not terminal-only as a tool, annotated read-only, " +
				"idempotent or destructive from its access, and brief, schema and status as resources. " +
				"A tool runs this executable with the given arguments in the given project directory and " +
				"returns its output and exit status. A command's standard input is empty, so a question " +
				"to a person is answered no. install registers this executable in Claude Code's user " +
				"configuration as \"bay\" through the claude command, only when the registration differs; " +
				"\"bay install\" does the same where claude is installed, and check reports a missing or stale registration. " +
				"Serving rewrites the agent skill when it differs from this executable's, so the skill follows an update.",
			Example:  "bay mcp install",
			Access:   Changes,
			Terminal: true,
		},
		{
			Name: "help", Args: "[command]",
			Summary:  "Print help for one command, or the command list.",
			Example:  "bay help up",
			Access:   Reads,
			Terminal: true,
		},
		{
			Name: "version", Args: "[--json]",
			Summary: "Print this executable's version, commit, Go version, platform and contract schema version.",
			Example: "bay version --json",
			Access:  Reads,
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
			"Local domain the project's service domains fall under. Naming it here pins it to the project."},
		{"x-bay.extra_domains", "list of string", "empty",
			"Further local domains the project's service domains may fall under."},
		{"x-bay.network", "string", "default",
			"Container network the project's services and the proxy share."},
		{"name", "string", "the Compose file's directory name",
			"Project name. Prefixes every container name."},
	}
}

// ServiceKeys are the keys under a service's `x-bay` mapping.
func ServiceKeys() []Key {
	return []Key{
		{"services.<name>.x-bay.domains", "list of string", "none: the service is internal",
			"Domains the proxy routes to this service, each under one of the project's domains. " +
				"Each domain gets its own certificate and route. A service without it runs and " +
				"other services reach it by name, but the proxy does not route to it and issues it no certificate."},
		{"services.<name>.x-bay.trust", "boolean", "true",
			"Whether the container trusts the certificates this machine trusts: the system's public roots and " +
				"this machine's authority, which issues the certificates of its projects and of its peers' names. " +
				"They are mounted read-only at /etc/bay/ca-bundle.crt and named by SSL_CERT_FILE, CURL_CA_BUNDLE, " +
				"REQUESTS_CA_BUNDLE and NODE_EXTRA_CA_CERTS; for Java a PKCS #12 store is mounted at /etc/bay/truststore.p12 " +
				"and named by JAVA_TOOL_OPTIONS. The container starts through /etc/bay/init, which adds the authority " +
				"(/etc/bay/ca.crt) to the image's own store and where update-ca-certificates and update-ca-trust read it, " +
				"then runs the image's or the file's entrypoint and command; nothing is mounted over the image's store, so " +
				"installing ca-certificates in the container works. A variable the service sets itself is left to the " +
				"service. false gives the container its image's store only."},
		{"services.<name>.x-bay.job", "boolean", "false",
			"Whether the service is a job: it runs to completion when a job is queued for it, with bay job add or its schedule, " +
				"one job at a time on the machine. up and start leave it alone, nothing may depend on it, and it serves no domain."},
		{"services.<name>.x-bay.schedule", "string", "none",
			"When the agent queues the job, once bay job schedule follows the project: five cron fields (minute hour day-of-month month " +
				"day-of-week, in the machine's local time) or @hourly, @daily, @weekly, @monthly, @yearly. A schedule that comes round " +
				"while its last job is queued or running queues nothing."},
		{"services.<name>.x-bay.timeout", "duration", "1h",
			"The longest one run of the job may take, up to 24h, such as 30m. At the limit the job is stopped with its stop signal and " +
				"grace period and recorded as failed."},
		{"services.<name>.x-bay.certificate", "boolean", "true",
			"Whether the container is given a certificate for its own name, issued by this machine's authority for " +
				"<project>-<service>.bay.test, the container name and the service's domains, usable as a server and as a client. " +
				"It is mounted read-only at /etc/bay/tls/cert.pem, its key at /etc/bay/tls/key.pem (readable by every user in the container) " +
				"and the authority at /etc/bay/tls/ca.pem, and reissued at up when 30 days or less remain. Redis takes them as --tls-cert-file, " +
				"--tls-key-file and --tls-ca-cert-file; MySQL and MariaDB as --ssl-cert, --ssl-key and --ssl-ca. PostgreSQL refuses a key other " +
				"users can read, so its command first copies the key to a file it owns: sh -c 'install -o postgres -m 600 /etc/bay/tls/key.pem " +
				"/tmp/key.pem && exec docker-entrypoint.sh postgres -c ssl=on -c ssl_cert_file=/etc/bay/tls/cert.pem -c ssl_key_file=/tmp/key.pem'. " +
				"A path the service mounts itself is left to the service. false gives no certificate."},
	}
}

// ServiceLabels are the labels this tool reads from a Compose service.
func ServiceLabels() []Key {
	return []Key{
		{"bay.port", "integer", "see the port order below",
			"Port the service listens on inside the container."},
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
		{"bay.domain", "string", "empty", "Domains the proxy routes to the container, separated by commas; empty for an internal service."},
		{"bay.scheme", "string", "http", "Scheme the proxy uses to reach the service."},
		{"bay.config", "string", "none", "Digest of the resolved service configuration and local image."},
		{"bay.volume", "string", "none", "Compose declaration key of a managed named volume."},
		{"bay.grace", "integer", "none", "Stop grace period of the service in whole seconds; set only when the Compose file sets stop_grace_period."},
		{"bay.stop_signal", "string", "none", "Signal that stops the service, one of the names in stop_signal; set only when the Compose file sets stop_signal."},
	}
}

// PortOrder lists where the container port is read from, first match wins.
func PortOrder() []string {
	return []string{
		"the bay.port label",
		"the first entry of `expose`",
		"the container side of the first entry of `ports`",
		"none: the service has no port and is not checked for connections; a service with a domain is refused",
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
		{"working_dir", "string", "image default", "Working directory of the process inside the container."},
		{"read_only", "boolean", "false", "Mount the container root filesystem read-only."},
		{"cap_drop", "list of string", "empty", "Linux capabilities to drop, including ALL."},
		{"tmpfs", "string or list of string", "empty", "In-memory mounts at absolute paths in the container, for data that must not outlive it. Docker takes mount options after a colon (/run:size=64m); Apple container takes the path alone and an option is rejected."},
		{"depends_on", "list or mapping", "empty", "Startup dependencies: service_started, service_healthy or service_completed_successfully. Unsupported options and cycles are rejected."},
		{"healthcheck", "mapping", "empty", "Startup CMD/CMD-SHELL test; interval (default 30s), timeout (30s), retries (default 3, from 1 to 100), start_period (0) and disable. " +
			"up, start and restart run it, as an exec in the container, right after the service starts and before the services that come after it in dependency order. " +
			"The first attempt runs at once, not after one interval. An attempt that fails or takes longer than timeout is a failure; " +
			"a failure while start_period has not passed since the start is not counted. The next attempt follows interval after a failure. " +
			"The service is healthy at the first attempt that succeeds; it fails after retries counted failures, or when its container stops, " +
			"or after start_period + retries x (interval + timeout) + 1s, and the command fails with the service's output. " +
			"The bound is in the file on purpose: a wait with no limit hides a service that never starts; a slow service gets a longer start_period. " +
			"There is no monitoring after startup."},
		{"container_name", "string", "<project>-<service>", "Container name."},
		{"networks", "list or mapping", "the project network", "First entry is used."},
		{"mem_limit", "string", "runtime default", "Memory limit."},
		{"cpus", "string", "runtime default", "CPU count."},
		{"stop_grace_period", "duration", "runtime default (5s)", "Time the container gets to stop before it is killed, as a whole number of seconds such as 30s or 1m30s. Stop, restart, down and replacement pass it to the runtime."},
		{"stop_signal", "string", "runtime default", stopSignalDescription()},
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
		{"One job at a time per machine, in the order jobs were queued.",
			"Two heavy runs on one machine compete for its memory; one after the other each gets it. A job keeps its result on the machine that ran it until it is read, so a link that drops loses nothing."},
		{"One proxy container per machine, named bay-edge.",
			"Projects share it. Its configuration is rebuilt from every running project."},
		{"Routes are read from container labels, not from Compose files.",
			"Removing a project's containers removes its routes without editing a file."},
		{"Domains are machine state, delegated once in /etc/resolver on macOS and named in a marked block of /etc/hosts elsewhere.",
			"A project inherits the machine default unless its Compose file pins one."},
		{"Certificates are issued and reissued automatically.",
			"One per routed domain, plus a default certificate for unrouted names."},
		{"Container addresses change on every start.",
			"The proxy names backends and resolves them per request, so addresses are never pinned."},
		{"up, down, start, stop and restart return after the proxy serves the new configuration.",
			"Reloading nginx is asynchronous, so the commands poll the proxy's health endpoint."},
		{"up, start and restart publish the route of a started service after the proxy connects to it.",
			"A healthcheck runs inside the service's container, so the proxy runs its own connection attempt to the service's address."},
		{"Only machine setup requires administrator rights: /etc/resolver entries and the relay daemon on macOS; the firewall, the agent, the hosts block and the system trust store elsewhere.",
			"Trusting the certificate authority on macOS uses the user's trust settings."},
		{"Every service container trusts this machine's authority unless its Compose file sets x-bay.trust: false.",
			"A program in a container verifies against its image's store, which does not hold the authority that issues the local names."},
		{"Every service is reached at <project>-<service>.bay.test, by other services and by this machine, on Apple container as on Docker.",
			"install sets Apple container's DNS domain to bay.test and Docker gives the same name as a network alias."},
		{"An interrupted up leaves nothing it created.",
			"On SIGINT or SIGTERM it removes the containers it created and keeps what existed before and every volume."},
		{"Machine setup is applied only by install, which changes nothing that is in place; check and install -dry-run report it without changing it.",
			"A setup step done by hand is not checked, repeated or removed by anything."},
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
		{"A job stays queued.", "A job queued before it runs, or the agent that runs the queue is not running.", "bay job list"},
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
		{"~/.bay/ca.crt, ca.key", "The certificate authority.", false},
		{"~/.bay/certs/", "Issued certificates.", false},
		{"~/.bay/conf.d/stack.conf", "The generated proxy configuration.", false},
		{"~/.bay/machine.json", "The machine's domains and settings.", false},
		{"~/.bay/groups.json", "Registered projects.", false},
		{"~/.bay/peers.json, peers/", "Approved peers and their authorities.", false},
		{"~/.bay/trust/", "The certificates service containers trust.", false},
		{"~/.bay/jobs/", "The job queue: one file per job, its output beside it, and the projects whose schedules are followed.", false},
		{"~/.claude/skills/bay/, ~/.agents/skills/bay/", "The agent skill, where those directories exist.", false},
		{"/etc/resolver/<domain>", "macOS: delegates the domain to the DNS server.", true},
		{"~/Library/LaunchAgents/dev.bay.dns.plist", "macOS: runs the DNS server.", false},
		{"/Library/LaunchDaemons/dev.bay.relay.plist", "macOS: runs the relay containers use to reach peers.", true},
		{"/etc/systemd/system/bay-agent.service", "Linux: runs the agent that keeps the hosts block and the peer document.", true},
		{"/etc/hosts (marked block)", "Linux: names the domains the proxy serves.", true},
		{"/etc/bay/firewall.nft, bay-firewall.service", "Linux: accepts new connections only from the hosts install -only-from named.", true},
		{"/usr/local/share/ca-certificates/bay.crt", "Linux: the authority in the system trust store; the path follows the system.", true},
		{"/etc/systemd/system/bay.slice", "Linux: holds the containers bay runs below the machine's memory; past 70% they slow, at 80% the kernel kills inside the slice only, and swap is held to 10%.", true},
		{"/etc/sysctl.d/99-bay.conf", "Linux: the free memory the kernel keeps for allocations that cannot wait, such as the network driver's, and when it starts reclaiming.", true},
	}
}
