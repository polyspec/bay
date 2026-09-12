# Machine state

Status: implemented.

This document lists everything `bay` creates outside its repository,
and how each item is removed.

## Files

| Path | Created by | Requires root | Removed by |
| --- | --- | --- | --- |
| `/etc/resolver/<domain>` | `install`, or `up` when a domain is new | Yes | `bay uninstall` |
| `~/Library/LaunchAgents/dev.bay.dns.plist` | `install` | No | `bay uninstall` |
| `~/.bay/ca.crt`, `ca.key` | First run that needs a certificate | No | Delete the directory |
| `~/.bay/certs/` | Each route | No | `Reissue all`, or delete the directory |
| `~/.bay/conf.d/stack.conf` | Each proxy sync | No | Regenerated on the next sync |
| `~/.bay/machine.json` | Domain changes | No | Delete the file |
| `~/.bay/groups.json` | Project registration | No | Delete the file |
| `~/.bay/logs/` | The launchd agent | No | Delete the directory |
| `~/.bay/completions/` | Successful foreground initializers | No | Delete the record; initialization then requires an explicit retry |
| `~/.bay/locks/` | Project lifecycle commands | No | Inactive files can remain; active locks release when the command ends |

## System state

| State | Set by | Requires root | Removed by |
| --- | --- | --- | --- |
| User trust setting for the certificate authority | `install` | No | `security remove-trusted-cert ~/.bay/ca.crt` |
| launchd user agent `dev.bay.dns` | `install` | No | `bay uninstall` |
| Appearance preference | The window's Auto/Dark/Light control | No | `defaults delete dev.bay.bar appearance` |

## Containers

| Name | Created by | Removed by |
| --- | --- | --- |
| `bay-edge` | Any command that produces routes | The last `down`, or `container rm -f bay-edge` |
| `<project>-<service>` | `up`, `start`, `restart` | `down` |

## Ownership

The machine setup is the resolver entries, the DNS agent and the proxy. Each of
the three exists once per machine and cannot be divided: a resolver entry is
keyed by domain and `/etc/resolver` is one directory per host, the agent is one
launchd job carrying one list of domains and one listening address, and the
proxy is one container name per engine.

`-state` selects the directory holding the authority, the certificates and the
project registry. It does not divide the machine setup, because the machine
setup cannot be divided. One state directory owns it.

The owner is the state directory the registered agent runs with, read from the
`-state` argument of `dev.bay.dns`. The owner is therefore read from
the program that answers the domains, and not from a record kept beside it that
could disagree with it. No agent registered means no owner, and the next
`install` takes it.

A command run from a state directory that is not the owner does not write,
replace or remove any part of the machine setup. It stops and names the owner.
`bay install` is the one command that takes ownership, and it says
what it took over.

Reading is not owning. `status`, `doctor` and every command that only reads run
from any state directory.

## Delegation

A domain is delegated by `install`, and by `up` when the domain is new.
Delegation writes `/etc/resolver/<domain>` and adds the domain to the agent.

A domain stops being delegated only when it is asked for: `bay domain
remove <name>` and `bay uninstall`. Both remove the resolver entry.

No other command removes a resolver entry. `up`, `down` and `sync` add what the
domains they serve are missing and remove nothing, so running a project never
withdraws a delegation that project did not ask about.

An entry `/etc/resolver` holds for a domain this machine does not delegate is
not removed. It is reported by `bay doctor`, because a file under
`/etc` that this tool did not write on this machine's behalf is not this tool's
to delete.

## Privilege

Writing under `/etc/resolver` requires root. Every other step runs as the user.

Root is needed once per domain, not once per command. A project served under a
domain that is already delegated needs no privileged step at all: the authority,
the certificates and the user's trust settings are all written as the user. A
command that asks for nothing privileged does not ask for a password.

The command line acquires root with `sudo` on the terminal it was started from.
The application has no terminal and calls
`AuthorizationExecuteWithPrivileges`, which presents the system authentication
panel; the privileged step is executed by the `bay` binary inside the
application bundle.
