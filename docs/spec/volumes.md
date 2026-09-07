# Named volumes

[한국어](volumes.ko.md) · [Lifecycle](service-lifecycle.md)

Managed and external named volumes are supported. Local verification does not install binaries or migrate existing application data.

Top-level `volumes` accepts a declaration key, optional `name`, `external`, `driver: local` and `driver_opts.size`. Unknown drivers and options are rejected. Managed names default to `<project>-<key>`. `size` is a positive byte count with an optional K, M, G, T or P suffix, using powers of 1024; absent size uses the runtime default. External declarations support only `external: true` and optional `name`, and must already exist.

Before starting containers, the tool checks every selected volume. A managed volume must carry the exact project, declaration key, volume-role and configuration labels; matching names alone do not grant ownership. Existing volumes must have the expected local driver and configured size. Mismatch is an error, never automatic adoption, resize or replacement. Missing managed volumes are created after all existing volume checks pass, under the project lifecycle lock. External volumes are checked for existence and are not relabelled.

`down` removes owned containers and preserves both managed and external volumes. Repeated `up` uses the same volume and data. Declaring another database, queue or cache requires Compose configuration, not application-specific volume creation code. Changing a managed volume's size requires an explicit operator-managed data migration outside this command; the tool will refuse the mismatched declaration.

Verification: `make check` and `go test -race ./internal/stack ./internal/contract` cover declaration validation, ownership, exact size, default names, unchanged reuse and external behavior. `BAY_SERVICE_E2E=1 go test -race ./internal/stack -run '^TestManagedVolumeActualRuntime$' -count=1 -timeout=120s` creates a unique 64 MiB volume and one container. It verifies data survives unchanged startup and container removal/recreation, rejects another project and resize, then removes only its owned fixture resources. Every preexisting container remains unchanged; shared proxy/DNS and application databases are not modified.
