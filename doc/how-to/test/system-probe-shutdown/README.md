# Reproduce slow uprobe shutdown

This scenario identifies time spent releasing USM-style uprobes, independently
of Kubernetes scheduling, cloud provisioning, or telemetry delivery. It uses
real kernel probes, the Agent's `UprobeAttacher` and `FileRegistry`, and the
repository's pinned `ebpf-manager`. Both cases use the implemented production attacher; the multi case also uses
the production program-graph loader with the same minimal eBPF program body.

It does not deploy a full Agent or send TLS traffic. It establishes attachment
lifecycle costs; it does not validate TLS payload capture or reproduce a
specific Bottlerocket kernel regression.

## Run

Requirements: Linux arm64 or amd64, Docker with permission to run privileged
containers, a C compiler, and the repository's Bazel development environment.
Use a development host. The container has no network or host PID namespace;
probes target only the private executable copied into that container. The
scenario does not run `Manager.Start()`, whose stale tracefs cleanup can inspect
unrelated host probes. `AddHook` attaches immediately on an initialized manager.

Choose a trusted image with glibc compatible with the host-built binaries and
the same CPU architecture. Pull the image first. Then, from the repository root:

```sh
python3 doc/how-to/test/system-probe-shutdown/run.py \
  --image <trusted-development-image> \
  --output /tmp/uprobe-shutdown-results \
  --counts 128,512 \
  --repetitions 2
```

The output directory must be new. The runner builds through Bazel, creates a
uniquely named container, runs cases **sequentially**, and removes that container
in a `finally` block. Do not run several copies concurrently: kernel tracing
locks are global, so concurrent teardown changes the timings.

Optional `--micro` also runs the C-only reproduction, comparing one worker,
12 workers, one-offset multi links, groups of 32, and one multi link containing
all offsets. Increasing counts to several thousand can make a single perf
shutdown take minutes. Every case has a timeout; container removal may also
wait for kernel cleanup after a timeout.

The tested local image was `devcontainer-sandbox-3230907649:latest`; use the
recorded image ID in `metadata.json` to identify that run precisely. This name
is local to the investigation workspace, not a published image recommendation.

## Measurements

The supervisor waits for a ready record after all probes are attached. It then
delivers either SIGTERM or SIGKILL directly to the test process, measures until
`waitpid` reaps it, and samples every thread's kernel stack every 250 ms. The
wait loop polls every 10 ms, so reported exit durations near 10 ms are bounded
by that resolution.

On SIGTERM, the Agent scenario also records:

1. Time inside `UprobeAttacher.Stop()`.
2. Time inside the subsequent `Manager.Stop()`.
3. A Go goroutine profile if shutdown lasts longer than a second.

SIGKILL bypasses the Go handler, so only the supervisor's wall time and kernel
stacks measure that path. Signal 9 plus a wait status of 9 is the expected
result for those cases. A nonzero exit for SIGTERM fails the scenario.

`results.jsonl` contains structured phase records. Per-case `.stdout` and
`.stacks` files retain raw output and stack samples. `metadata.json` records
kernel, architecture, source commit and image ID; `build.log` records the build.

The executable contains distinct, valid NOP instruction offsets that are never
executed by the scenario. Each offset is attached to the same minimal eBPF
program in one binary. This deliberately isolates teardown; the one-group
multi result is a favorable grouping case. Real USM workloads have several
programs and binaries, so they still need several groups.

## Interpretation

For perf attachment, expect the Go stack to contain:

```text
UprobeAttacher.Stop (waiting)
  deferred FileRegistry.Clear
  detachFromBinary
  Manager.DetachHook
  Probe.Stop / link Close
```

The stopping goroutine and the registry-cleanup goroutine are separate. The
profile records both; the indentation above describes their dependency.

Kernel stacks can contain `perf_event_detach_bpf_prog`,
`perf_trace_event_unreg`, `perf_uprobe_destroy` and synchronization functions.
After SIGKILL, `task_work_run` and `do_exit` identify process-exit cleanup.

Linux 6.18's `perf_uprobe_destroy()` holds the global `event_mutex` while
unregistering the perf trace event, including its synchronization wait. This
limits gains from adding userspace workers. Multi-uprobe links avoid this
perf-event path and unregister all offsets within a link before synchronizing.

On the investigation host (Linux 7.0.0-1014-aws, arm64), both graceful and forced
exit reproduced slow cleanup. Treat those results as confirmation of the
mechanism, not proof of a Bottlerocket 6.18 regression. Run this same matrix on
the affected kernel and retain full customer stacks before closing CONS-12089.

See [the implementation](PROPOSAL.md) for backend selection, resource ownership,
and remaining rollout validation.

## Implementation results (18 November 2026)

Two sequential repetitions on Linux 7.0.0-1014-aws, arm64, produced these
signal-to-reap ranges. All 16 cases passed.

| Offsets | Backend | SIGTERM | SIGKILL |
| --- | --- | --- | --- |
| 128 | Perf ioctl | 9.62–10.00 s | 10.62–10.90 s |
| 128 | Implemented multi-uprobe | 10.90–11.00 ms | 10.37–10.42 ms |
| 512 | Perf ioctl | 39.45–40.55 s | 39.38–40.84 s |
| 512 | Implemented multi-uprobe | 10.96–11.04 ms | 10.35–10.42 ms |

The 512-offset case improves signal-to-reap latency by more than 99.97% for both
signals. The scenario verifies 512 perf-event FDs versus one multi-uprobe link.
Almost all graceful perf shutdown time was inside attacher Stop. Multi attacher
Stop took approximately 5–7 ms; the subsequent collection/manager cleanup was
below 1 ms. Fast exits are bounded by the supervisor's 10 ms polling resolution.

## Real HTTPS validation

`TestUSMUprobeMultiHTTPS` sends real Go and native OpenSSL HTTPS requests through
the production USM programs and HTTP decoders, comparing perf and multi backends.
The Go HTTP/2 case exercises the larger decoder tail-call graph. It verifies that
the multi collection actually loaded, rather than silently accepting fallback,
and records full USM stop time after capture. In one local sample per case:

| Traffic | Perf USM Stop | Multi USM Stop | Reduction |
| --- | --- | --- | --- |
| Go HTTP/1 | 1.73 s | 0.15 s | 91% |
| Go HTTP/2 | 1.77 s | 0.23 s | 87% |
| Native OpenSSL | 1.60 s | 0.83 s | 48% |

All six capture cases passed. These are local integration checks,
not fakeintake or Kubernetes rollout tests.

The test requires privileged Linux, built CO-RE USM/shared-library assets,
Python with SSL, and an **unstripped** test binary (`bazel build --strip=never`).
Run the repeatable container setup:

```sh
python3 doc/how-to/test/system-probe-shutdown/run_https.py \
  --image <trusted-development-image> \
  --output /tmp/usm-https-results
```

It uses a private PID/network namespace, root-owned copied assets, and read-only
tracefs. Empty read-only views of the legacy `kprobe_events` and `uprobe_events`
files prevent Manager.Start's stale-event cleanup from touching host probes.
Actual kernel attachment, TLS capture, tail calls and shutdown remain exercised.
