# Troubleshooting

Start with `bay doctor`, which reports the machine setup and changes
nothing, and `bay status`, which reports what is running.

| Symptom | Cause | Action |
| --- | --- | --- |
| A name does not resolve | The DNS agent is not registered | `bay install` |
| A name does not resolve, and the agent is registered | The answer was cached while the proxy was down | Wait a few seconds and retry |
| 502 from the proxy | The container runs but does not listen on the resolved port | `bay logs <service>`, then check the port order in `bay schema` |
| 404 from the proxy | No route claims the name: the service is stopped, marked internal, or the domain differs | `bay status` |
| Certificate warning on a routed name | The certificate authority is not trusted | `bay install` |
| Certificate warning on an unrouted name | A wildcard whose parent is a single label is rejected by clients | Use a two-label domain such as `shop.test` |
| A command asks for the password | The project uses a domain the machine does not delegate yet | Expected; it is asked once per domain |
| The browser still fails after a fix | The browser cached the certificate | Open a new window |

## Reading the state from another program

```sh
bay status --json
```

The output carries the machine state, every registered project, each service's
state and address, and the certificate list.

## When the proxy is missing

The proxy is created by any command that produces routes, and removed when the
last route is withdrawn. `bay up` in any project recreates it.

## Removing everything

`docs/operations/install.md` lists the removal steps.
