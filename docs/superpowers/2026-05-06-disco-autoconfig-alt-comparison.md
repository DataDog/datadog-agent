# Configuration Discovery for Agent Integrations: PoC Comparison

## Status

Two PoCs implementing automatic configuration of agent integrations from container service info have been built and exercised end-to-end against krakend.

- **PoC A** (`vitkyrka/disco-autoconfig`): new `discover()` classmethod on Python check classes called by AD via a dedicated Python bridge.
- **PoC B** (`vitkyrka/disco-autoconfig-alt`): existing `check()` callback overloaded to handle a synthetic config carrying service info; trial-mode runner suppresses errors during probing.

Both PoCs pass `krakend/tests/test_e2e.py::test_e2e_discovery` (agent picks up krakend container, probes 9090, emits `krakend.api.*` metrics). PoC B's e2e was confirmed by directly running `ddev env test --dev krakend py3.13-2.10 -- -k test_e2e_discovery` on `vitkyrka/disco-autoconfig-alt` (commit `bc5c4e05747`) on 2026-05-06. PoC A's e2e was confirmed during its construction on `vitkyrka/disco-autoconfig`.

Both PoCs share merge-base `80e785f4d0ec` with `origin/main`.

## Quantitative comparison

### datadog-agent

Measured via `git diff --shortstat origin/main..<branch>` over the touched subsystems (`comp/core/autodiscovery/`, `pkg/collector/python/`, `pkg/collector/worker/`, `rtloader/`):

| | PoC A | PoC B |
|---|---|---|
| Files changed | 29 | 22 |
| Lines inserted | +1353 | +437 |
| Lines deleted | -103 | -97 |
| Net LOC delta | +1250 | +340 |

Direct diff PoC A → PoC B: 28 files changed, +358 / -1268 (PoC B removes ~910 net LOC from PoC A's baseline).

**New files created (datadog-agent):**

PoC A creates 10 new production files:
- `comp/core/autodiscovery/discoverer/cache.go` (110 LOC)
- `comp/core/autodiscovery/discoverer/discoverer.go` (164 LOC)
- `comp/core/autodiscovery/discoverer/python_bridge.go` (25 LOC)
- `comp/core/autodiscovery/discoverer/python_bridge_nopython.go` (16 LOC)
- `comp/core/autodiscovery/discoverer/types.go` (53 LOC)
- `pkg/collector/python/discover.go` (107 LOC — cgo bridge)
- New rtloader symbols: `run_discover` in `rtloader/three/three.cpp` (~46 LOC added) and `rtloader/include/rtloader.h`, `rtloader/include/datadog_agent_rtloader.h`, `rtloader/common/builtins/datadog_agent.c`

PoC B creates 2 new production files:
- `comp/core/autodiscovery/autodiscoveryimpl/trial.go` (49 LOC)
- `pkg/collector/worker/trial.go` (38 LOC — callback registry)

### integrations-core

Measured via `git diff --shortstat origin/master..<branch>` over `datadog_checks_base/datadog_checks/base/checks/openmetrics/`, `datadog_checks_base/datadog_checks/base/utils/discovery/`, `krakend/`:

| | PoC A | PoC B |
|---|---|---|
| Files changed | 16 | 16 |
| Lines inserted | +457 | +433 |
| Lines deleted | -5 | -4 |

The integrations-core delta is nearly identical. Both PoCs add the same utility library (`discovery/http.py`, `ports.py`, `service.py`, `tcp.py`, `verifiers.py`). The difference is concentrated in:
- PoC A adds `_bridge.py` (Python helper called from rtloader) and the `discover()` classmethod (~20 LOC) in `openmetrics/v2/base.py`.
- PoC B adds ~40 LOC `__init__`/`check()`/`_configure_from_discovery` override directly in `krakend/datadog_checks/krakend/check.py`; the base class gets only a 6-line `__discovery_service__`-awareness stub in `openmetrics/v2/base.py` (optional convenience, not load-bearing).

## Architectural differences

**Python entry points.** PoC A adds `_run_discover` in `datadog_checks_base/datadog_checks/base/utils/discovery/_bridge.py` as a fixed entry point called by name from `rtloader/three/three.cpp:509`. It also adds `run_discover` as a new rtloader C API symbol (`rtloader/include/datadog_agent_rtloader.h`, `rtloader/include/rtloader.h`, `rtloader/common/builtins/datadog_agent.c`). Any integration that defines `DISCOVERY_PORT_HINTS` or overrides `discover()` goes through this path. PoC B adds no new rtloader symbol; the existing `run_check` path handles everything.

**AD goroutine.** PoC A adds `discoveryRetryLoop` (`autoconfig.go:822`), a 5 s ticker that calls `cfgMgr.retryPendingDiscoveries()` in a loop. The `AutoConfig` struct gains a `discoveryRetryStop chan struct{}` field. PoC B adds no goroutine; the check runner's existing tick at `min_collection_interval` (default 15 s) is the retry mechanism.

**AD mutex held during probes.** PoC A: `resolveTemplateForService` (called with `cm.m` held — see `configmgr.go` comment "This method must be called with cm.m locked") calls `cm.discoverer.Discover` (`configmgr.go:494`), which crosses into Python synchronously. Every HTTP probe — including retries triggered by `discoveryRetryLoop` → `retryPendingDiscoveries` → `resolveTemplateForService` — blocks the configmgr mutex for the duration of the probe (typically <1 s, but up to several seconds on timeout). With many services this serializes all AD operations behind each probe. PoC B: `resolveTemplateForService` (`configmgr.go:420`) builds a synthetic config synchronously (dict construction, JSON marshal, secrets decrypt — microseconds) and returns. The probe runs later in the check runner goroutine with no AD lock held.

**Multi-instance support.** PoC A's bridge already returns a `list[dict]` from `discover()`. The current single-instance gate is an explicit `if len(result.Configs) > 1 { log.Warnf(...) }` at `configmgr.go`; removing it is a one-line change. PoC B is structurally single-instance per `(service, integration)` pair because it schedules one check per service — each check carries one instance.

**Error suppression.** PoC A never schedules the check until `discover()` returns a config, so the normal error reporting path is never triggered by a probe failure. PoC B adds ~22 LOC in `pkg/collector/worker/worker.go:213-232` and `pkg/collector/worker/trial.go` to detect `IsTrialMode()` checks, suppress `integration_errors`, suppress the service check, suppress `log.Error`, and notify AD. The suppression uses a type assertion (`interface{ IsTrialMode() bool }`) which is safe: non-trial checks are unaffected because the assertion fails.

**State machinery.** PoC A's `discoverer/cache.go` tracks 4 states per `(svcID, integration)`: `stateMiss`, `stateHit`, `statePending`, `stateGivenUp`. The cache enforces the retry schedule (2×5s + 8×30s = 250s window; give up on the 11th failure) and prevents re-probing until `nextRetryAt`. Total: `cache.go` 110 LOC + `discoverer.go` 164 LOC + `types.go` 53 LOC. PoC B's `trial.go` (49 LOC) tracks a single integer per `checkid.ID`: consecutive failure count. After 5 consecutive failures AD unschedules the check. Retry timing is whatever `min_collection_interval` is configured to — default 15 s, giving ~75 s to 5 failures.

## Tradeoffs

| Concern | PoC A (`discover()`) | PoC B (`check()` reuse) |
|---|---|---|
| New Python rtloader symbol | Yes — `run_discover`/`_run_discover` in rtloader C and Python | None |
| New Go package | `comp/core/autodiscovery/discoverer/` (~460 LOC, 5 files) | None |
| New agent-side files total | 10 new production files | 2 new production files |
| New AD goroutine | `discoveryRetryLoop` (5 s ticker) | None |
| AD mutex held during probe | Yes — every probe (initial + retries) blocks all AD | No — synthetic config built in microseconds; probe runs in runner |
| Concurrent probes | Serialized behind `cm.m` | Parallel up to runner pool size |
| Retry window | Bespoke schedule: 2×5s + 8×30s = 250s (10 retry slots, give up on 11th failure) | Free — `min_collection_interval` × threshold (default 75s at 15s×5) |
| Multi-instance support | Structurally extensible (1-line gate removal) | Structurally single-instance per service per integration |
| Error suppression | Not needed | ~22 LOC in worker; silent on failure but auditable |
| Per-integration code (krakend) | `DISCOVERY_PORT_HINTS = [9090]` — 1 line added to existing class | `__init__` + `check()` + `_configure_from_discovery` — ~40 LOC new methods |
| Integration testability | `KrakendCheck.discover(mock_service)` — pure function, no instance needed | Instantiate check with mock config, call `check()`, inspect scraper state |
| Integration migration cost | Low: set `DISCOVERY_PORT_HINTS` on any `OpenMetricsBaseCheckV2` subclass | Medium: implement `check()` override + endpoint-injection pattern each time |

## Recommendation

Ship PoC B if multi-instance discovery is not a near-term requirement; revisit if it becomes one.

The primary operational argument for PoC B is the mutex. PoC A holds `cm.m` across every HTTP probe — initial and every retry. At scale (hundreds of containers per host) this serializes `processNewService`, `processDelService`, and config-reload events behind each probe. The issue is called out in PoC A's own design doc (`specs/2026-05-06-discover-retry-design.md`) as a known non-goal and deferred. PoC B eliminates the problem structurally: the synthetic config is built in microseconds and the probe runs without any lock.

PoC B's agent-side surface is substantially smaller: 2 new files vs 10, +340 net LOC vs +1250, no new rtloader C API, no new goroutine. The integrations-core delta is identical (both require the same utility library). The per-integration authoring overhead is real — ~40 LOC vs 1 line — but it is bounded, explicit, and follows a copy-paste pattern. For a small set of integrations (krakend, boundary, cockroachdb are already seeded) this is not a blocking concern.

The error-suppression machinery in `worker.go` is the only PoC B code that requires ongoing audit. The type assertion is opt-in and does not change behavior for non-trial checks. The suppression paths are narrow (integration_errors, log.Error, service check) and covered by the e2e test. The `IsTrialMode()` interface should be documented so future worker refactors know it is a stability contract, not dead code.

Multi-instance (druid, tekton, torchserve) is the one place PoC A is structurally ahead. PoC A's bridge returns `list[dict]` today; PoC B would need a design to handle >1 discovered instance per service (one option: schedule N checks, one per discovered endpoint; another: allow list-returning `_configure_from_discovery`). If multi-instance is required in the next quarter, the PoC B approach warrants a design spike before committing to it.

## Code references

### datadog-agent

**PoC A key files:**
- `comp/core/autodiscovery/discoverer/discoverer.go` — Discoverer interface, Discover() impl, `defaultRetrySchedule`
- `comp/core/autodiscovery/discoverer/cache.go` — 4-state cache with `nextRetryAt`
- `comp/core/autodiscovery/discoverer/python_bridge.go` — cgo bridge to `run_discover` C symbol
- `pkg/collector/python/discover.go` — `RunDiscover()` Go wrapper over rtloader
- `rtloader/three/three.cpp:509` — `Three::runDiscover` C++ implementation
- `comp/core/autodiscovery/autodiscoveryimpl/autoconfig.go:822` — `discoveryRetryLoop` goroutine
- `comp/core/autodiscovery/autodiscoveryimpl/configmgr.go:494` — `cm.discoverer.Discover()` call site (inside `cm.m` lock)

**PoC B key files:**
- `comp/core/autodiscovery/autodiscoveryimpl/trial.go` — `trialRegistry`: consecutive-failure counter, 49 LOC
- `pkg/collector/worker/trial.go` — `RegisterTrialResultCallback` / `notifyTrialResult`, 38 LOC
- `comp/core/autodiscovery/autodiscoveryimpl/configmgr.go:420` — synthetic config builder, `TrialMode=true` set here
- `pkg/collector/worker/worker.go:213` — `IsTrialMode()` detection, error suppression, `notifyTrialResult` call
- `comp/core/autodiscovery/autodiscoveryimpl/autoconfig.go` — `trialRegistry` wired to `RegisterTrialResultCallback`; `RecordTrialResult` on unschedule

### integrations-core (shared between both PoCs)

- `datadog_checks_base/datadog_checks/base/utils/discovery/http.py` — `http_probe()`
- `datadog_checks_base/datadog_checks/base/utils/discovery/verifiers.py` — `is_prometheus_exposition()`
- `datadog_checks_base/datadog_checks/base/utils/discovery/ports.py` — `candidate_ports()`
- `datadog_checks_base/datadog_checks/base/utils/discovery/service.py` — `Service` / `Port` dataclasses

**PoC A only:**
- `datadog_checks_base/datadog_checks/base/utils/discovery/_bridge.py` — `_run_discover()` entry point
- `datadog_checks_base/datadog_checks/base/checks/openmetrics/v2/base.py` — `discover()` classmethod + `DISCOVERY_PORT_HINTS`/`DISCOVERY_METRICS_PATH` class vars

**PoC B only:**
- `krakend/datadog_checks/krakend/check.py` — `__init__` endpoint-injection, `check()` override, `_configure_from_discovery()` (~40 LOC new)
