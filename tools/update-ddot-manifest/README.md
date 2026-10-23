# update-ddot-manifest

`update-ddot-manifest` rewrites the pinned component versions in the DDOT (Datadog Distribution of
OpenTelemetry) OCB manifest (`comp/otelcol/collector-contrib/impl/manifest.yaml`) to match a target
Datadog Agent release.

It resolves versions from a **static compatibility table** (`DDOT_VERSIONS` in `versions.go`) — no
network access — so a given Agent version always produces the same result. **Custom components**
(modules outside the OpenTelemetry core/contrib and Datadog module prefixes) are detected and
reported, but left untouched: their compatibility with a new release can only be verified by their
maintainer.

## Why not just search-and-replace the version?

The manifest pins two distinct version streams: OTel core modules that reached 1.0 are on `v1.x`
(e.g. `go.opentelemetry.io/collector/confmap/provider/envprovider v1.65.0`), while core beta modules
and all contrib modules are on `v0.x` (e.g. `.../receiver/otlpreceiver v0.159.0`). The tool applies
`CoreStableVersion` to components currently on `v1.x` and `ContribVersion` to the rest.

## Build

```bash
cd tools/update-ddot-manifest
go build .
```

(or `bazel build //tools/update-ddot-manifest`)

## Usage

```
update-ddot-manifest --agent-version <version> [--manifest <path>] [--output <path>] [--dry-run]
```

| Flag | Required | Default | Description |
|---|---|---|---|
| `--agent-version` | yes | — | Target Agent version; the key into `DDOT_VERSIONS` (e.g. `7.83.2`) |
| `--manifest` | no | `comp/otelcol/collector-contrib/impl/manifest.yaml` | OCB manifest to read |
| `--output` | no | same as `--manifest` | Where to write the updated manifest |
| `--dry-run` | no | false | Report changes and custom components without writing |

## Example

```bash
./update-ddot-manifest --agent-version 7.83.2 \
  --manifest ../../comp/otelcol/collector-contrib/impl/manifest.yaml --dry-run
```

## Scope

The tool only rewrites `manifest.yaml`. Regenerating `go.mod` / `components.go` from the updated
manifest is a separate step handled by `dda inv collector.generate`. Updating the rest of the repo's
pinned OTel versions remains the job of `tasks/collector.py`.

## Maintaining the compatibility table

Each DDOT release adds a row to `ddotVersions` in `versions.go` with its OCB version, core-stable
version and contrib version. The table is seeded from the otelcol-byoc toolkit's `DDOT_VERSIONS`.
