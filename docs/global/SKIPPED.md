# Skipped packages

Packages that were intentionally not documented, with reasons.

| Package | Reason |
|---|---|
| `pkg/proto/pbgo` | Pure generated protobuf code |
| `pkg/trace/pb` | Pure generated protobuf code (contains only a single `ToStringSlice` helper over `protoiface.MessageV1`; all types live in `pkg/proto/pbgo/trace`) |
| `pkg/util/compression/impl-noop` | Trivial no-op stub |
| `pkg/collector/rustchecks` | Directory is completely empty — no Go source files, no stubs, nothing to document |
| `pkg/util/xc` | Single exported function (`GetSystemFreq`) wrapping `sysconf(_SC_CLK_TCK)` via CGo; not imported anywhere else in the codebase |

## Batch: security/secl, util, clusteragent (2026-03-26)

No packages were skipped in this batch. All five requested packages were documented:

| Package | Output file |
|---------|-------------|
| `pkg/security/secl/rules` | `pkg/security/secl-rules.md` |
| `pkg/util/procfilestats` | `pkg/util/procfilestats.md` |
| `pkg/util/gpu` | `pkg/util/gpu.md` |
| `pkg/clusteragent/admission` | `pkg/clusteragent/admission.md` |
| `pkg/clusteragent/autoscaling` | `pkg/clusteragent/autoscaling.md` |

`pkg/util/procfilestats` and `pkg/util/gpu` are small (3 files, ~50-60 lines of logic each) but were included because they have non-obvious platform-conditional behavior and are used by multiple callers across the codebase.
