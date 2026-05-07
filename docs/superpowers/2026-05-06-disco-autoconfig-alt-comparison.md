# Configuration Discovery for Agent Integrations: PoC Comparison

## Status

Two PoCs implementing automatic configuration of agent integrations from container service info have been built and exercised end-to-end across nine integrations.

- **PoC A** (`vitkyrka/disco-autoconfig`): new `discover()` classmethod on Python check classes called by AD via a dedicated rtloader symbol + Python bridge.
- **PoC B** (`vitkyrka/disco-autoconfig-alt`): existing check class instantiated normally with a synthetic config carrying service info; an `AgentCheck.__new__` dispatch routes such instances through a `_TrialModeProxy` that iterates `target_cls.generate_configs(service)` and commits the first candidate whose `check()` runs without an error.

Both PoCs share merge-base `80e785f4d0ec` with `origin/main`.

### E2E status (PoC B, all PASS as of 2026-05-07)

`test_e2e_discovery` runs against the ddev e2e environment and asserts metrics/service-checks are emitted via the discovery flow:

- krakend, boundary, cockroachdb, n8n, pulsar, ray, temporal — `test_e2e_discovery` PASS.
- kuma — runs in a `kind` cluster; would need separate alt-PoC plumbing in conftest.
- kong — no `test_e2e.py` exists in the repo.

Beyond the success-path e2e, PoC B was also exercised against the `krakend-delayed` smoke (krakend container sleeps 60 s before listening, simulating the AD startup race). After fixing two bugs surfaced by the smoke (see "Findings" below), the trial-mode flow behaves as designed: probe failures during the 60 s sleep are emitted at DEBUG (`trial-mode check krakend:... failed (suppressing integration error)`), do not increment `integration_errors`, do not produce a critical service-check, and once krakend listens the next check tick succeeds and metrics flow normally.

## Quantitative comparison

### datadog-agent

Measured via `git diff --shortstat origin/main..<branch>` over the touched subsystems (`comp/core/autodiscovery/`, `pkg/collector/python/`, `pkg/collector/worker/`, `comp/collector/`, `rtloader/`):

|  | PoC A | PoC B |
|---|---|---|
| Files changed | 29 | 23 |
| Lines inserted | +1353 | +447 |
| Lines deleted | −103 | −97 |
| Net LOC delta | +1250 | +350 |

Direct diff PoC A → PoC B: PoC B removes ~900 net LOC from PoC A's baseline.

**New files created (datadog-agent):**

PoC A creates 10 new production files:
- `comp/core/autodiscovery/discoverer/cache.go` (110 LOC)
- `comp/core/autodiscovery/discoverer/discoverer.go` (164 LOC)
- `comp/core/autodiscovery/discoverer/python_bridge.go` (25 LOC)
- `comp/core/autodiscovery/discoverer/python_bridge_nopython.go` (16 LOC)
- `comp/core/autodiscovery/discoverer/types.go` (53 LOC)
- `pkg/collector/python/discover.go` (107 LOC — cgo bridge)
- New rtloader symbols: `run_discover` in `rtloader/three/three.cpp` (~46 LOC) plus declarations in headers and `rtloader/common/builtins/datadog_agent.c`

PoC B creates 2 new production files:
- `comp/core/autodiscovery/autodiscoveryimpl/trial.go` (49 LOC)
- `pkg/collector/worker/trial.go` (38 LOC — callback registry)

### integrations-core

Measured via `git diff --shortstat origin/master..<branch>` over `datadog_checks_base/`, `datadog_checks_dev/`, and the nine seeded integrations:

|  | PoC A | PoC B |
|---|---|---|
| Files changed | 67 | 67 |
| Lines inserted | +832 | +845 |
| Lines deleted | −48 | −52 |

The integrations-core deltas are essentially identical. Both PoCs add the same `auto_conf_discovery.yaml` markers, the same `--discovery-min-instances` / `--discovery-timeout` test infrastructure, and the same shared utilities (`Service`/`Port` dataclasses, `candidate_ports`). The mechanism difference falls out cleanly:

- PoC A adds a `_bridge.py` rtloader entry helper (54 LOC) and a `discover()` classmethod (~13 LOC) on `OpenMetricsBaseCheckV2`.
- PoC B adds an `__new__` + `_TrialModeProxy` dispatch (~80 LOC) in `AgentCheck` and a `generate_configs` classmethod (~15 LOC) on `OpenMetricsBaseCheckV2`. It also removes the `is_prometheus_exposition()` / `http_probe()` verifier helpers' role in the discovery path — they remain in `utils/discovery/` but are no longer load-bearing (the actual scraper is the verifier).

### Per-integration adoption cost (vs `origin/master`)

|  Integration | PoC A check.py LOC | PoC B check.py LOC |
|---|---|---|
| krakend | 0 | 0 |
| n8n | 0 | 0 |
| pulsar | 1 (`DISCOVERY_PORT_HINTS = [8080]`) | 1 (same) |
| ray | 1 (`[8080]`) | 1 (same) |
| temporal | 1 (`[8000]`) | 1 (same) |
| kuma | 1 (`[5680]`) | 1 (same) |
| cockroachdb V2 | 2 (port hint + path) | 2 (same) |
| boundary | 5 (port hint + classmethod `discover()` override for `health_endpoint`) | 8 (port hint + classmethod `generate_configs()` override) |
| kong wrapper | 1 (delegate to V2 in `discover()`) | 5 (`__discovery_service__` clause in `__new__`, drop legacy `discover()`) |
| cockroachdb wrapper | 1 (same) | 6 (same) |

The per-integration story is **back to PoC A's level**. The earlier alt-PoC iteration (with placeholder URLs + `__init__` overrides + `_post_discovery_hook` + cached_property invalidation) added ~40 LOC to krakend and similar amounts to other integrations; that approach was discarded in favour of the `__new__` + proxy mechanism, which lets each candidate go through a fresh, full `__init__` of the target class — the cached state and config-model that integrations rely on simply work, no per-integration accommodation needed.

## Architectural differences

**Python entry points.** PoC A adds `_run_discover` in `datadog_checks_base/.../discovery/_bridge.py` as a fixed entry point called by name from `rtloader/three/three.cpp`. It also adds `run_discover` as a new rtloader C API symbol (`rtloader/include/datadog_agent_rtloader.h`, `rtloader/include/rtloader.h`, `rtloader/common/builtins/datadog_agent.c`). PoC B adds zero rtloader symbols; the existing `run_check` path handles everything.

**AD goroutine.** PoC A adds `discoveryRetryLoop`, a 5 s ticker that calls `cfgMgr.retryPendingDiscoveries()`. The `AutoConfig` struct gains a `discoveryRetryStop` channel. PoC B adds zero goroutines; the check runner's existing tick at `min_collection_interval` is the retry mechanism.

**AD mutex held during probes.** PoC A's `resolveTemplateForService` is called with `cm.m` held and synchronously invokes `cm.discoverer.Discover` → Python `discover(service)` → HTTP probes. Every probe — initial and retries — holds the configmgr mutex. With many services this serializes all AD operations behind each probe (typically <1 s, but up to several seconds on timeout). PoC B's `resolveTemplateForService` builds the synthetic config in microseconds and returns; the actual scrape happens later in the runner with no AD lock held.

**Probe-vs-scraper asymmetry.** PoC A's `discover()` uses `is_prometheus_exposition()` (or a similar verifier) to decide if a candidate endpoint is "Prometheus enough"; the scraper that runs later may have stricter requirements. The verifier and scraper can drift over time. PoC B's trial proxy runs the actual scraper as the verifier — the first candidate whose `check()` completes without raising is the winner. There is no asymmetry possible.

**Multi-instance support.** PoC A's bridge already returns a `list[dict]` from `discover()`; the current single-instance gate is an explicit one-line warn-and-truncate at `configmgr.go:508`. Removing it requires a small scheduling-loop change. PoC B is structurally single-instance per `(service, integration)` pair: the proxy commits one winning candidate config and runs it forever. Supporting >1 instance per service would require either schedule-N-checks-per-service (a Go-side change) or a fan-out inside one `check()` call (not supported by the AgentCheck contract today).

**Error suppression.** PoC A never schedules the check until `discover()` returns a config, so the normal error reporting path is never triggered by a probe failure. PoC B adds ~22 LOC in `pkg/collector/worker/worker.go` and `pkg/collector/worker/trial.go` to detect `IsTrialMode()` checks, suppress `integration_errors`, suppress the service check, suppress `log.Error`, and notify AD via `RecordTrialResult`. The suppression uses a type assertion (`interface{ IsTrialMode() bool }`) — non-trial checks are unaffected because the assertion fails.

**State machinery.** PoC A's `discoverer/cache.go` tracks 4 states per `(svcID, integration)`: `stateMiss`, `stateHit`, `statePending`, `stateGivenUp`. The cache enforces a fixed retry schedule (2×5s + 8×30s = 250 s, give up on the 11th failure) and prevents re-probing until `nextRetryAt`. Total: 327 LOC across `cache.go` + `discoverer.go` + `types.go`. PoC B's `trial.go` (49 LOC) tracks a single integer per `checkid.ID`: consecutive failure count. After 5 failures, AD unschedules the check. Retry timing is `min_collection_interval × threshold` (default 75 s at 15 s × 5).

**Per-integration code surface.** PoC A places its discovery hook on the integration's check class (`@classmethod discover(cls, service)`). PoC B places it on the integration's check class (`@classmethod generate_configs(cls, service_dict)`). Same shape, slightly different signature (PoC B yields candidate complete instance dicts rather than returning a list). Boundary's "derive a related field from the resolved endpoint" pattern is a `super().<method>()` + post-processing override in both PoCs — same lines of code.

## Tradeoffs

| Concern | PoC A (`discover()`) | PoC B (`__new__` + `generate_configs`) |
|---|---|---|
| New rtloader C symbol | Yes (`run_discover`) | None |
| New Go package | `comp/core/autodiscovery/discoverer/` (~460 LOC, 5 files) | None |
| New agent-side production files | 10 | 2 |
| Net agent-side LOC vs main | +1250 | +350 |
| New AD goroutine | `discoveryRetryLoop` (5 s ticker) | None |
| AD mutex held during probe | Yes — every probe blocks all AD | No — synthetic config built in microseconds |
| Concurrent probes | Serialized behind `cm.m` | Parallel up to runner pool size |
| Probe verifier matches scrape semantics | No (separate `is_prometheus_exposition` etc.) | Yes (the scraper IS the verifier) |
| Retry window | Bespoke schedule: 2×5s + 8×30s = 250 s | Free — `min_collection_interval × threshold` (default 75 s) |
| Multi-instance support | Structurally extensible (1-line gate removal) | Structurally single-instance per service |
| Error suppression | Not needed | ~22 LOC in worker; silent on failure but auditable |
| Per-integration code (typical) | 0–1 lines (port hint) | 0–1 lines (port hint) |
| Per-integration code (boundary-style outlier) | ~5 lines (`super().discover()` + post-process) | ~5 lines (`super().generate_configs()` + post-process) |
| Integration unit-test ergonomics | `Cls.discover(mock_service)` — pure-function | Instantiate `Cls(name, init_config, [trial_instance])`, run, inspect |

## Findings from running the PoC end-to-end

These were not anticipated by the original design but became apparent when the implementation was exercised against real test environments. They are PoC B-specific (PoC A has its own different set of edge cases, but those weren't surfaced because PoC A's per-integration adoption is shallower).

### 1. `middleware.CheckWrapper` doesn't forward `IsTrialMode` (datadog-agent fix)

`collectorImpl.RunCheck` wraps every check in a `middleware.CheckWrapper` (`comp/collector/collector/collectorimpl/internal/middleware/check_wrapper.go`) before handing it to the scheduler. The wrapper only forwards methods of the `check.Check` interface. The worker's anonymous-interface assertion `check.(interface{ IsTrialMode() bool })` therefore returned `ok=false` for the wrapped check, and the suppression branch never fired — failures were logged at ERROR and counted as integration errors. Fixed at `aa7f243d622` (10 LOC) by forwarding `IsTrialMode()` from the wrapper. **Lesson:** any layer between the loader and the worker that doesn't forward the trial-mode contract silently breaks suppression. There is exactly one such layer today; future ones must be considered.

### 2. rtloader's "no subclasses" rule prevents dynamic proxy classes (Python finding)

An earlier draft of PoC B's proxy was a dynamically-generated subclass `(_TrialModeMixin, target_cls)`. This worked for the first instantiation, but rtloader's check-class detector at `three.cpp:727` skips any AgentCheck subclass that itself has subclasses ("Agent integrations are supposed to have no subclasses"). After the first instantiation, `target_cls.__subclasses__()` returned the dynamic proxy class, causing the loader to fail to find `target_cls` on subsequent instantiations of the same integration. Replaced with a single fixed `_TrialModeProxy(AgentCheck)` that stashes `target_cls` as an instance attribute. **Lesson:** Python-level metaprogramming on AgentCheck subclasses interacts with the rtloader's class-detection heuristic in non-obvious ways. Stay leaf-class.

### 3. `__new__` returning a non-subclass instance skips `__init__` (Python language gotcha)

`_TrialModeProxy` is intentionally not a subclass of `target_cls` (per finding 2). Python's normal `__new__` → `__init__` chaining only fires `__init__` if the returned instance's class is a subclass of `cls`. Without that, `__init__` was silently skipped, so `_winner` was never set — the first `proxy.run()` raised `AttributeError`. Fixed by calling `proxy.__init__(*args, **kwargs)` explicitly in `__new__` before returning.

### 4. `dd_agent_check` surfaces per-instance errors even with multi-match discovery (test-framework fix)

Auto-discovery's `ad_identifiers` may match multiple containers (ray's case: head + 3 workers serve `/metrics`, but 2 task-runner containers using the same image don't). The agent's check command exits 0 once `discovery_min_instances` configs are *scheduled* — but the test fixture (`replay_check_run`) raised on any per-instance error. With trial-mode, errors from non-metric containers are expected. Added an `ignore_errors` parameter to `replay_check_run` and have `dd_agent_check` set it when `discovery_min_instances` is passed. **Side effect:** the test's metric/service-check assertions are now the source of truth, which is the right semantics for multi-match discovery.

### 5. `discovery_min_instances` is a *scheduling* threshold, not a *success* threshold

The agent's `WaitForConfigsFromAD` waits until `discovery_min_instances` configs are *scheduled* by AD; it then runs all scheduled configs once and returns. If the first-scheduled is a non-metric container, it runs (fails), and the agent exits before the head/worker containers have been scheduled. ray's `test_e2e_discovery` was therefore racy with `discovery_min_instances=1`. The test now uses `discovery_min_instances=6, discovery_timeout=60` to wait for all six matches. A more robust fix would be at the agent level (count *successful* runs, not scheduled configs), but that is out of scope for this PoC comparison.

## Recommendation

Updated from the previous version of this doc. The original recommendation was conditional on whether multi-instance support was a near-term need. After the AgentCheck-level redesign, the per-integration cost gap that was the main concession of PoC B has closed: per-integration code is now matched line-for-line with PoC A.

**Ship PoC B.** The deciding factors:

1. **No AD mutex blocking.** PoC A holds `cm.m` across every HTTP probe; at scale this serializes container events behind probe latency. PoC B does the synthetic-config build in microseconds; the actual scrape happens in the runner with no AD lock. This is a real operational gain that grows with deployment size, and PoC A's own design doc calls it out as a known non-goal.
2. **No probe/scraper asymmetry.** PoC B uses the actual scraper as the verifier; PoC A uses a separate `is_prometheus_exposition()` predicate. Over time the predicate drifts from what the scraper actually accepts, leading to "the probe says yes but the scrape errors forever" failure modes. PoC B is structurally immune.
3. **Smaller agent-side surface.** 2 new files vs. 10, +350 net LOC vs. +1250, no new rtloader C symbol, no new goroutine. Lower long-term maintenance load.
4. **Per-integration cost matches PoC A.** Most integrations need 0–1 LOC. The boundary outlier needs ~5 LOC in both PoCs (same shape: `super().<method>()` + post-process).

Multi-instance support remains the one place PoC A is structurally ahead. PoC A's bridge returns `list[dict]` today; flipping the single-instance gate is a one-line change. PoC B would need a design spike to handle >1 instance per service. If multi-instance is a near-term requirement (druid, tekton, torchserve), do the design spike before merging — but the trial-mode dispatch via `AgentCheck.__new__` is independently sound and the spike's outcome (e.g., yielding multiple winners from `generate_configs` and scheduling each) doesn't invalidate the architecture.

Two operational items that should land alongside or shortly after a PoC B merge:

- **Document `IsTrialMode()` as a stability contract** in the SDK and worker code so future refactors don't silently break suppression (per finding 1).
- **Consider tightening `discovery_min_instances` semantics** at the agent level to count successful runs rather than scheduled configs (per finding 5). This would let test authors use `discovery_min_instances=1` and rely on the agent to wait for at least one *useful* discovery, rather than tuning per integration.

## Code references

### datadog-agent

**PoC A key files:**
- `comp/core/autodiscovery/discoverer/discoverer.go` — Discoverer interface, `Discover()` impl, `defaultRetrySchedule`
- `comp/core/autodiscovery/discoverer/cache.go` — 4-state cache with `nextRetryAt`
- `comp/core/autodiscovery/discoverer/python_bridge.go` — cgo bridge to `run_discover` C symbol
- `pkg/collector/python/discover.go` — `RunDiscover()` Go wrapper over rtloader
- `rtloader/three/three.cpp:509` — `Three::runDiscover` C++ implementation
- `comp/core/autodiscovery/autodiscoveryimpl/autoconfig.go:822` — `discoveryRetryLoop` goroutine
- `comp/core/autodiscovery/autodiscoveryimpl/configmgr.go:494` — `cm.discoverer.Discover()` call site (inside `cm.m` lock)

**PoC B key files:**
- `comp/core/autodiscovery/autodiscoveryimpl/trial.go` — `trialRegistry`: consecutive-failure counter
- `pkg/collector/worker/trial.go` — `RegisterTrialResultCallback` / `notifyTrialResult`
- `comp/core/autodiscovery/autodiscoveryimpl/configmgr.go:420` — synthetic config builder, `TrialMode=true`
- `pkg/collector/worker/worker.go:213` — `IsTrialMode()` detection, error suppression, `notifyTrialResult` call
- `comp/collector/collector/collectorimpl/internal/middleware/check_wrapper.go` — `IsTrialMode()` forwarder (added)
- `comp/core/autodiscovery/autodiscoveryimpl/autoconfig.go` — `trialRegistry` wired to `RegisterTrialResultCallback`; `RecordTrialResult` on unschedule

### integrations-core

Shared between both PoCs:
- `datadog_checks_base/datadog_checks/base/utils/discovery/service.py` — `Service` / `Port` dataclasses
- `datadog_checks_base/datadog_checks/base/utils/discovery/ports.py` — `candidate_ports()`
- Each integration's `data/auto_conf_discovery.yaml` and the `--discovery-*` flags in `datadog_checks_dev/.../e2e/docker.py`

**PoC A only:**
- `datadog_checks_base/datadog_checks/base/utils/discovery/_bridge.py` — `_run_discover()` rtloader entry
- `datadog_checks_base/datadog_checks/base/utils/discovery/{http,verifiers,tcp}.py` — probe helpers used by `discover()`
- `datadog_checks_base/datadog_checks/base/checks/openmetrics/v2/base.py` — `discover()` classmethod

**PoC B only:**
- `datadog_checks_base/datadog_checks/base/checks/base.py` — `AgentCheck.__new__`, `_TrialModeProxy`, default `generate_configs`
- `datadog_checks_base/datadog_checks/base/checks/openmetrics/v2/base.py` — `generate_configs` classmethod, `DISCOVERY_PORT_HINTS` / `DISCOVERY_METRICS_PATH` class attrs
- `datadog_checks_dev/datadog_checks/dev/_env.py` and `.../plugin/pytest.py` — `replay_check_run` `ignore_errors` parameter

**Boundary-style outlier (both PoCs, similar shape):**

PoC A:
```python
@classmethod
def discover(cls, service):
    instances = super().discover(service)
    if instances:
        for instance in instances:
            base = instance['openmetrics_endpoint'].rsplit('/', 1)[0]
            instance['health_endpoint'] = f"{base}/health"
    return instances
```

PoC B:
```python
@classmethod
def generate_configs(cls, service_dict):
    for cfg in super().generate_configs(service_dict):
        base_url = cfg["openmetrics_endpoint"].rsplit('/', 1)[0]
        cfg["health_endpoint"] = f"{base_url}/health"
        yield cfg
```
