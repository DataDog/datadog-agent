<!--
Progress note for reducing the host-profiler dependency footprint on
go.opentelemetry.io/ebpf-profiler.
-->

# Host-Profiler eBPF Profiler Footprint Reduction Progress

## Summary

The plan has been implemented through the meaningful runtime seam:

1. Phase 1 is done: tracer parsing was localized.
2. Phase 2 is done: host-profiler now uses a local runtime fork with curated
   interpreter registration.
3. Phase 3 is partially done: the dependency-closure change is verified and a
   current Linux binary artifact was produced, but a comparable baseline binary
   size was not completed yet.
4. Phase 4 is not implemented and is not realistic in this workspace without
   additional BPF build/test tooling.

## What Was Completed

### Phase 1: Local tracer parsing

- Added local tracer parsing in
  [tracertypes.go](./tracertypes/tracertypes.go) and
  [tracertypes_test.go](./tracertypes/tracertypes_test.go).
- Updated the host-profiler receiver config to use local tracer parsing in
  [config.go](./collector/impl/receiver/config.go).
- Preserved the host-profiler-specific behavior:
  - comma-separated parsing
  - `all`
  - ignoring `native`
  - disabling `go` by default
  - enabling `labels` only when `collect_context` is on
  - arm64 `dotnet` filtering

### Phase 2: Curated runtime fork

- Added a local runtime fork under
  [ebpfprofiler](./ebpfprofiler).
- Switched the live receiver to the local fork in
  [factory.go](./collector/impl/receiver/factory.go) and
  [config.go](./collector/impl/receiver/config.go).
- Forked the runtime path:
  - local collector bridge
  - local controller
  - local tracer
  - local process manager
  - local exec info manager
- Curated interpreter registration in
  [manager.go](./ebpfprofiler/processmanager/execinfomanager/manager.go):
  - `apmint` stays always on
  - `golabels` remains conditional on `labels`
  - the upstream Go interpreter loader is no longer linked
- Added focused tests in
  [manager_test.go](./ebpfprofiler/processmanager/execinfomanager/manager_test.go)
  for the curated loader behavior.
- Added `# Missing` summaries at the top of copied files that still contain
  upstream `TODO:` comments, per repo guidance.

### Phase 3: Verification completed so far

- Verified targeted tests:

```text
go test ./comp/host-profiler/tracertypes
go test ./comp/host-profiler/ebpfprofiler/tracer/types
go test ./comp/host-profiler/ebpfprofiler/processmanager/execinfomanager
go test ./comp/host-profiler/ebpfprofiler/processmanager
GOOS=linux GOARCH=amd64 go test ./comp/host-profiler/ebpfprofiler/collector ./comp/host-profiler/ebpfprofiler/internal/controller ./comp/host-profiler/ebpfprofiler/tracer
```

- Verified dependency-closure improvement:

```text
GOOS=linux GOARCH=amd64 go mod why go.opentelemetry.io/ebpf-profiler/processmanager/execinfomanager
# (main module does not need package)

GOOS=linux GOARCH=amd64 go mod why go.opentelemetry.io/ebpf-profiler/interpreter/go
# (main module does not need package)
```

- Verified the `cmd/host-profiler` closure now contains the local exec info
  manager and no longer contains upstream `interpreter/go`.

## Current Measured Artifact

A current Linux `host-profiler` artifact exists in:

- `.tmp/host-profiler-size/current`
- `.tmp/host-profiler-size/current.stripped`

Current measured sizes:

- unstripped: `188307968` bytes
- stripped: `137586752` bytes

These are current-state numbers only. There is not yet a matching committed
baseline artifact to compute the final before/after delta.

## What Still Needs To Be Done

### Remaining Phase 3 work

1. Build a clean `HEAD` baseline Linux binary with the same command and compare
   it to the current artifact.
2. Record the resulting size delta, ideally both stripped and unstripped.
3. Keep a short note with the measurement commands and numbers for future
   review.

Suggested measurement flow:

```text
1. Create a temporary worktree at HEAD.
2. Build both baseline and current in a linux/amd64 Go container with:
   - trimpath
   - GOFLAGS=-p=1
   - GOMAXPROCS=1
   - GOGC=20
3. Compare the byte sizes of the two binaries.
```

### Cleanup and documentation

1. Add or update any user-facing docs that mention tracer support so the Go
   tracer behavior is explicit:
   host-profiler accepts `tracers: go`, but this binary does not provide local
   Go unwinding.
2. Decide whether to keep a checked-in note for the dependency/binary
   measurement or just include it in the PR description.
3. Consider a higher-level regression test for the receiver/config path if a
   Linux build environment is available in CI.

### Phase 4 status

Phase 4 is **not implemented**.

It is not realistic in this workspace right now because the local fork still
depends on upstream `support`, and the required BPF generation/test toolchain
is missing. The main missing pieces are:

- local ownership of the `support` package and generated assets
- `clang-17`, `llvm-link-17`, `llvm-strip-17`, `llc-17`, `clang-format-17`
- `qemu-system-x86_64` / `qemu-system-aarch64`
- `bluebox`
- `busybox`
- the upstream kernel test image setup

That means the practical next milestone is to finish the baseline/current size
comparison for the current Phase 1+2 work, not to start asset slimming here.

## Current Risk Notes

- The primary intended footprint cut is in place, but the exact binary delta is
  still unreported until the baseline build finishes.
- Phase 2 intentionally changes support behavior for local Go unwinding.
- The host-profiler receiver path is wired to the local runtime fork, so future
  upstream profiler updates will no longer automatically flow through this seam.
