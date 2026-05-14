# Upstream schema shims

This directory ships `config.schema.yaml` files for upstream OpenTelemetry
Collector components that don't yet have one published upstream at the
manifest's pinned version. Each subdirectory mirrors an upstream
`go.opentelemetry.io/collector/...` path (with `/` flattened to `_`) and is
treated by the bundler as if it lived in that cached module.

The bundler consults this directory before reading from the Go module cache
(see `_shim_schema_path` in `tasks/libs/otelcol_schema/bundle.py`). Refs into
or out of a shimmed schema resolve through the same namespace-relative + 
package-type machinery used for real cached upstream schemas, so a shim's
`/config/confighttp.client_config` ref still picks up `confighttp` from the
real Go module cache.

## Current shims

All come from the in-flight upstream PR
<https://github.com/open-telemetry/opentelemetry-collector/pull/15300> (branch
`feat/component-config-schemas` at <https://github.com/truthbk/opentelemetry-collector>).

| Shim directory | Upstream go path |
|---|---|
| `exporter_debugexporter/` | `go.opentelemetry.io/collector/exporter/debugexporter` |
| `exporter_otlpexporter/` | `go.opentelemetry.io/collector/exporter/otlpexporter` |
| `exporter_otlphttpexporter/` | `go.opentelemetry.io/collector/exporter/otlphttpexporter` |
| `receiver_otlpreceiver/` | `go.opentelemetry.io/collector/receiver/otlpreceiver` |
| `processor_batchprocessor/` | `go.opentelemetry.io/collector/processor/batchprocessor` |
| `processor_memorylimiterprocessor/` | `go.opentelemetry.io/collector/processor/memorylimiterprocessor` |
| `extension_zpagesextension/` | `go.opentelemetry.io/collector/extension/zpagesextension` |
| `internal_memorylimiter/` | `go.opentelemetry.io/collector/internal/memorylimiter` |

## Lifecycle

When the upstream PR merges and the manifest's pinned Collector version
catches up, each shim becomes redundant. To retire one:

1. Bump the manifest version (or wait for the existing version's cache to
   include the new schema).
2. Delete the shim's entry from `UPSTREAM_SHIMS` in
   `tasks/libs/otelcol_schema/bundle.py`.
3. Delete the shim subdirectory.
4. Run `dda inv otelcol-schema.gen` to refresh the bundle artifact — the
   output should be byte-identical to the pre-deletion state if the upstream
   schema matches the shim.

`dda inv otelcol-schema.inventory` is the running scoreboard for which
components still need shims (or upstream schemas).
