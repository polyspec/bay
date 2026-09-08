# Read-only diagnostics

[한국어](diagnostics.ko.md) · [Command contract](cli.md).

`status`, its JSON output, `doctor`, and the window's machine snapshot only read
state. They do not register or refresh a Compose project, create directories or
certificates, read a CA private key, change trust or resolver settings, or install
the DNS agent. Missing or unreadable public certificates are reported.
JSON includes `certificates.authority.readError` when the public CA cannot be
read. A readable public certificate does not prove its signing key is usable.

`status` lists registered projects. Selecting an unregistered Compose file does
not add it to machine state. `doctor` may include its domains when describing the
setup needed for a later `up`, without saving that selection.

`up`, `install`, and explicit registration in the window retain their existing
registration and setup effects. Read-only diagnostics do not perform those
actions. A caller requiring a prepared machine checks the reported CA trust,
pending setup steps, DNS state and proxy state before changing its own services.

Tests use isolated machine directories and fixture executables for container,
trust and launchd queries. They verify absent state remains absent, existing
files and modification times remain unchanged, Compose selection does not alter
registration, and public certificate information works without a private key.
