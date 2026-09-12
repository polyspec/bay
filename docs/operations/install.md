# Install

One-time setup for a machine. Everything after this runs without administrator
rights.

Requirements: macOS with Apple `container` 1.3 or later, and Go 1.27 or later.
The application also needs the Xcode command line tools, which supply the
compiler for its AppKit layer.

## Command line only

```sh
go install github.com/polyspec/bay/cmd/bay@latest
go install github.com/polyspec/bay/cmd/bayd@latest
```

Both binaries land in the same directory, which is what `bay install`
needs: it registers the DNS agent with the path of the `bayd` beside
`bay`.

## Command line and application

```sh
git clone https://github.com/polyspec/bay.git
cd bay
make
sudo make install
```

`make` produces `bin/bay`, `bin/bayd` and
`bin/baybar.app`. `make install` copies the two binaries to
`/usr/local/bin` and the application to `/Applications`.

Install elsewhere with `PREFIX` and `APPDIR`, which needs no `sudo` when the
directories belong to you:

```sh
make install PREFIX="$HOME/.local" APPDIR="$HOME/Applications"
```

`make uninstall` removes the installed files. It does not undo the machine
setup; run `bay uninstall` for that.

## Prebuilt downloads are not offered

A downloaded binary carries the quarantine attribute, and macOS terminates an
unsigned quarantined binary before it runs. Distributing a working download
requires an Apple Developer ID signature and notarization, which this project
does not have. Both paths above build on the machine, so nothing is
quarantined.

## Set up the machine

Open the application and select **Finish setup**, or run:

```sh
bin/bay install
```

Three steps are applied:

| Step | Administrator rights |
| --- | --- |
| Write `/etc/resolver/<domain>` for each delegated domain | Required |
| Add the certificate authority to the user's trust settings | Not required |
| Register the DNS server as a launchd user agent | Not required |

The command line asks for the password with `sudo` on the terminal it was
started from. The application presents the system authentication panel and runs
the privileged step through the `bay` binary inside its bundle.

## Verify

```sh
bin/bay doctor
```

The command reports what is still missing and changes nothing. A machine that
is set up prints `nothing to do`.

## Remove

```sh
bay uninstall
security remove-trusted-cert -d ~/.bay/ca.crt
rm -rf ~/.bay
sudo make uninstall          # in the clone, if make install was used
```

`uninstall` removes the resolver entries and the launchd agent. The certificate
authority stays in the trust settings until it is removed with the second
command. `docs/spec/machine-state.md` lists every path this tool creates.

## One machine, one setup

`-state` selects the directory holding the authority, the certificates and the
project registry. The resolver entries, the DNS agent and the proxy are the
machine's and there is one of each, so one state directory owns them.
`bay doctor` names the owner. A command run from another state
directory reads the machine but does not change its setup; it stops and says
whose it is. Take the setup over deliberately with `bay -state <dir>
install`.

## What asks for a password

Writing under `/etc/resolver` needs administrator rights. Nothing else does,
and it is needed once per domain rather than once per command: a project served
under a domain that is already delegated runs with no prompt at all. Run
`bay doctor` to see what is outstanding before running anything that
would ask.

Delegating a new domain from a script or from a program with no terminal cannot
ask for a password. Run `bay install` once from a terminal; every
command after that needs nothing.
