# Checklist

[Korean](checklist.ko.md)

- [o] CT-1 Stop an owned running service before replacement. Priority: service replacement correctness. Acceptance: tracked RED verifies stop before non-force removal, stopped service removal and no removal after a stop failure; run owner checks and actual HTTPS replacement without restarting the consuming edge. Preserve existing volumes and unrelated projects.

CT-1 also requires Apple native service names to match the running container addresses before lifecycle success. Native event subscription and installation are verified under CT-1-1; document requests are not retried.

Completion evidence: tracked RED cases for forced removal and dependent startup; GREEN replacement and hostname cases; `make check`; actual HTTPS requests after two service replacements, without an edge or DNS restart. CLI installed from clean source 052e567; address and HTTPS verification passed after installation.

  - [o] CT-1-1 Replace repeated DNS lookups with native address events. Priority: enforce event use without changing the address equality criterion. Evidence: DNSServiceGetAddrInfo emits removal and addition events during real service replacement. Acceptance: subscribe once, process address additions and removals, cancel and collect subscription cleanup on readiness, error or deadline; run owner RED/GREEN and actual HTTPS replacement without resetting the edge or DNS. Record CLI installation separately. Depends on: CT-1.

CT-1-1 completion evidence: one-subscription RED/GREEN, native C/header build-input RED/GREEN, address addition/removal and cancellation cases, `make check`, related race checks and actual HTTPS requests after replacing both services without an edge or DNS restart. Native event CLI installation remains a separate deployment record.

  - [o] CT-1-1-1 Record native event CLI deployment. Acceptance: verify clean implementation revision in the installed executable and run actual address and HTTPS replacement cases for both services without restarting edge or DNS. Evidence: installed CLI revision `95d73d4`; both cases pass. DNS executable and menu application are unchanged. Depends on: CT-1-1.

  - [o] CT-1-1-2 Make hostname readiness independent of a negative answer cached before the container existed. Priority: `up` failed for a new service after 60 seconds when its name had been looked up before its container existed. Cause: the runtime DNS answers NXDOMAIN without SOA for a name that does not exist yet, the system resolver keeps that answer for 77 seconds, and a DNSServiceGetAddrInfo subscription is answered from that entry and does not receive negative answers. Containers resolve through the network gateway, which answers from the same cache, so they cannot resolve the name either. Acceptance: readiness sends the question to the runtime DNS server, the server's answer replaces the cached entry, and the readiness deadline is unchanged. Evidence: RED `BAY_E2E=1 go test ./internal/stack -run TestNativeNameSubscriptionReplacesCachedNegativeAnswer` failed after its 20-second bound with `received []`; GREEN with long-lived DNSServiceQueryRecord queries the addresses arrive after 10 ms and the system resolver returns them afterwards; `make check`. Depends on: CT-1-1.
