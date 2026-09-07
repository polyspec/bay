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

## Privilege

Writing under `/etc/resolver` requires root. Every other step runs as the user.

The command line acquires root with `sudo` on the terminal it was started from.
The application has no terminal and calls
`AuthorizationExecuteWithPrivileges`, which presents the system authentication
panel; the privileged step is executed by the `bay` binary inside the
application bundle.
