# Grouped USM TLS uprobes

USM TLS attachments use multi-uprobe links when the kernel feature probe succeeds.
Set `service_monitoring_config.enable_uprobe_multi: false` (or
`DD_SERVICE_MONITORING_CONFIG_ENABLE_UPROBE_MULTI=false`) to select the perf backend.
Unsupported kernels retain perf attachments. Permission, preparation, and load
failures also retain perf, with a warning identifying the failure.

## Implementation

- The final manager instruction patcher captures TLS program specs after constant
  editing and telemetry patches, retaining CO-RE metadata.
- A second collection loads the TLS roots and reachable protocol decoders with
  `AttachTraceUprobeMulti`, using the original kernel BTF and loader options.
- Non-program-array maps are shared with the original collection. Program arrays
  are private: Linux requires matching expected attach types across tail calls.
  Routes are populated before any multi-uprobe link can execute.
- Go, native OpenSSL/GnuTLS, Istio and Node.js attachers receive the loaded programs
  before startup. They attach one owned link per file identity, eBPF program and
  entry/return semantics. Existing device/inode reference counting keeps links
  alive until the last process using the file exits.
- Go manual return offsets remain ordinary uprobes at return instructions; native
  uretprobes use `UretprobeMulti`.
- Registry cleanup closes every link, including rollback after partial attachment.
  Protocol attachers stop before the second collection and its private arrays close.
- Existing probe counters count locations; `uprobe_attacher.multi_links__created`
  counts created groups separately.

The original program collection remains available for fallback. Loading the
second graph adds program and private-array memory; data maps are shared. Memory
and CPU overhead should be measured under representative production workloads.

Grouping cannot combine different binaries or eBPF program bodies. The one-link
scale benchmark is a favorable grouping case, not a production latency promise.

## Validation

See [the reproduction guide](README.md) for repeatable signal-to-reap timing and
real HTTPS capture checks. The implemented backend reduced 512-offset SIGTERM and
SIGKILL exits from approximately 39–41 seconds to 10–12 milliseconds on the test
host. Tests verify actual perf/link FD counts and successful Go/native TLS capture.

Before rollout, run on Bottlerocket 6.18 and x86_64, exercise Istio/Node.js and
additional protocol decoders, and compare representative CPU/memory usage. These
checks are distinct from the completed local mechanism and traffic validation.
