# Checklist

[Korean](checklist.ko.md)

- [o] CT-1 Stop an owned running service before replacement. Priority: service replacement correctness. Acceptance: tracked RED verifies stop before non-force removal, stopped service removal and no removal after a stop failure; run owner checks and actual HTTPS replacement without restarting the consuming edge. Preserve existing volumes and unrelated projects.

CT-1 also requires Apple native service names to match the running container addresses before lifecycle success. Native event subscription and installation are verified under CT-1-1; document requests are not retried.

Completion evidence: tracked RED cases for forced removal and dependent startup; GREEN replacement and hostname cases; `make check`; actual HTTPS requests after two service replacements, without an edge or DNS restart. CLI installed from clean source 052e567; address and HTTPS verification passed after installation.

  - [o] CT-1-1 Replace repeated DNS lookups with native address events. Priority: enforce event use without changing the address equality criterion. Evidence: DNSServiceGetAddrInfo emits removal and addition events during real service replacement. Acceptance: subscribe once, process address additions and removals, cancel and collect subscription cleanup on readiness, error or deadline; run owner RED/GREEN and actual HTTPS replacement without resetting the edge or DNS. Record CLI installation separately. Depends on: CT-1.

CT-1-1 completion evidence: one-subscription RED/GREEN, native C/header build-input RED/GREEN, address addition/removal and cancellation cases, `make check`, related race checks and actual HTTPS requests after replacing both services without an edge or DNS restart. Native event CLI installation remains a separate deployment record.
