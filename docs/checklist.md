# Checklist

[Korean](checklist.ko.md)

- [o] CT-1 Stop an owned running service before replacement. Priority: service replacement correctness. Acceptance: tracked RED verifies stop before non-force removal, stopped service removal and no removal after a stop failure; run owner checks and actual HTTPS replacement without restarting the consuming edge. Preserve existing volumes and unrelated projects.

CT-1 also requires Apple native service names to match the running container addresses before lifecycle success. DNS has no exposed change event, so bounded readiness lookups are permitted and must report mismatches and timeout; document requests are not retried.

Completion evidence: tracked RED cases for forced removal and dependent startup; GREEN replacement and hostname cases; `make check`; actual HTTPS requests after two service replacements, without an edge or DNS restart. CLI built, not installed.
