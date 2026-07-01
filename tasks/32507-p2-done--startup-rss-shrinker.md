# Startup RSS Shrinker

## Context

Revive the old RSS shrinker idea from PR #32507 as a deliberately cosmetic, low-risk RSS presentation optimization. Customers and operators often judge Agent memory footprint using RSS-oriented tools (`top`, `ps`) even though PSS/cgroup metrics are better measures of real memory pressure. After startup, the Agent may have touched clean file-backed pages that are no longer part of the active working set; a single delayed reclamation pass can let Linux page those out and improve visible RSS without attempting to change the steady-state memory model.

## Goals

- Add a one-shot startup RSS shrinker that runs **2m30s after process startup**.
- Make it best-effort and low-noise.
- Reuse the existing file-backed pageout implementation currently duplicated in the trace loader and installer daemon.
- Add an env-var kill switch for safety.
- Add optional `malloc_trim` support behind a separate env var for measurement only.
- Avoid adding persistent config knobs.

## Non-goals

- No recurring timer / periodic shrink loop.
- No attempt to find a perfect lifecycle-ready hook in every daemon.
- No claim that this primarily improves PSS or cgroup memory.
- No default-on `malloc_trim`.

## Proposed implementation

1. Create or extend a shared internal helper, likely under `cmd/internal/rssshrinker`.
2. Extract the current Linux file-backed `MADV_PAGEOUT` logic from:
   - `cmd/loader/memory_linux.go`
   - `cmd/installer/subcommands/daemon/memory_linux.go`
3. Expose a small API such as:
   - `Schedule(delay time.Duration)`
   - `Shrink()` or unexported shrink implementation
4. Schedule a one-shot shrink from the major long-running Agent binaries near startup:
   - core agent
   - cluster-agent
   - process-agent
   - security-agent
   - system-probe
   - trace-agent
   - otel-agent
   - consider dogstatsd / installer / private action runner only if appropriate after inspecting their startup paths
5. Use a fixed delay of `2*time.Minute + 30*time.Second`.
6. Add disablement env var, e.g. `DD_STARTUP_RSS_SHRINKER_DISABLED=true`.
7. Add optional malloc trim env var:
   - `DD_STARTUP_RSS_SHRINKER_MALLOC_TRIM=true`
   - Implement only for `linux && cgo`.
   - Provide safe no-op behavior for `linux && !cgo` and non-Linux builds.
8. Keep unsupported-kernel and per-mapping failures best-effort:
   - avoid stderr output during startup;
   - do not fail process startup;
   - treat unsupported `MADV_PAGEOUT` as no-op;
   - continue past individual mapping failures where reasonable.

## Validation plan

- Build using `dda inv`, not raw `go` commands:
  - `dda inv agent.build --build-exclude=systemd`
  - relevant component builds for touched binaries.
- Run targeted unit tests if helper parsing/error handling is unit-testable.
- Rely on CI automated performance testing for detailed memory metrics, including RSS/PSS/cgroup behavior and startup/runtime regressions.
- Manually sanity-check a Linux Agent process where practical:
  - before/after `/proc/$PID/status` RSS fields;
  - `/proc/$PID/smaps_rollup` (`Rss`, `Pss`, clean/dirty breakdown);
  - absence of noisy logs/errors;
  - behavior with disable env var set;
  - behavior with optional malloc trim env var set.

## Risks and mitigations

- **Startup/runtime latency from page refaults**: one-shot delayed pass only, no periodic timer; CI perf testing should catch regressions.
- **Unsupported kernels**: best-effort no-op behavior.
- **cgo build fallout from `malloc_trim`**: isolate behind `linux && cgo`; default disabled; no-op elsewhere.
- **Short-lived CLI command overhead**: delayed trigger means most short-lived commands exit before the shrink runs.
- **Perceived metric mismatch**: PR description should explicitly state this primarily optimizes RSS presentation, not PSS/cgroup memory.

## PR description notes

Frame the change honestly as an RSS presentation optimization:

> Many users inspect Agent memory footprint with RSS-oriented tools even though PSS/cgroup memory are more accurate for real pressure. This adds a single delayed, best-effort Linux reclamation pass after startup to page out clean file-backed pages and return Go heap pages to the OS. The pass is disabled via env var and optional malloc trimming is separately opt-in for measurement.

Suggested env var names may be adjusted during implementation if repository conventions suggest a better naming pattern.
