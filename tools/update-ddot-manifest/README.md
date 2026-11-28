# update-ddot-manifest

`update-ddot-manifest` rewrites the pinned component versions in the DDOT (Datadog Distribution of
OpenTelemetry) OCB manifest (`comp/otelcol/collector-contrib/impl/manifest.yaml`) to match a target
Datadog Agent release.

It resolves versions from **static, hardcoded tables** (`versions.go` and `components.go`) — no
network access — so a given Agent version always produces the same result. **Custom components**
(modules not listed in the components catalog and outside the Datadog module prefix) are detected and
reported, but left untouched: their compatibility with a new release can only be verified by their
maintainer.

## Why not just search-and-replace the version?

Each component falls into one of four cells for a given release — {core, contrib} × {stable (`v1.x`),
unstable (`v0.x`)} — and the four version numbers can coincide, so the correct value cannot be
guessed from the version already written in the manifest. Instead, `components.go` records each
module's stream (`core`/`contrib`) and its **first stable version** (from the upstream release
history). For the requested release the tool writes the stable version when the module has reached
stability by then, and the unstable version otherwise (e.g. when an older release, from before the
module stabilized, is requested).

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

## Maintaining the hardcoded tables

Two in-repo, network-free tables drive the tool:

- `ddotVersions` (`versions.go`): one row per DDOT release with its OCB version and the four stream
  versions (core stable/unstable, contrib stable/unstable). Add a row for every release.
- `components` (`components.go`): one row per core/contrib module with its stream and first stable
  version, derived from the module's upstream OTel release history (empty = never stable). Keep it in
  sync as modules graduate to `v1.x`, and extend it to cover any core/contrib module the manifest may
  reference.
