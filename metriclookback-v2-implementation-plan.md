# Metric Lookback V2 Implementation Plan

## Goal

Implement the metric lookback V2 shadow pipeline as a stack of small PRs that can land on `main`, using `qbranch-1hz` as reference material where useful without carrying forward the old separate shadow scheduler design.

This plan intentionally keeps lookback storage, retention, trigger, dump, and backend flush work as a parallel workstream owned by someone else. The shadow pipeline owns the lookback sender manager/sender and should depend on a narrow writer interface for storage so both streams can progress independently.

V2 keeps one shared Agent scheduling timeline and splits at scheduler job dispatch:

```text
AutoConfig
  -> CheckScheduler derives normal check and optional shadow check
  -> one Scheduler owns timing for both
  -> scheduler job dispatch routes normal vs shadow checks
  -> runner normal channel/workers/backend sender
  -> runner shadow channel/workers/lookback sender interface
```

The key behavior is: shadow checks may block or apply backpressure on the shadow path, but must not block the normal check execution lane after scheduler job dispatch.

## Diagram Contracts

These are the contracts the diagram should show. They are the stable boundaries between boxes, independent of exact implementation details inside each package.

### Autodiscovery To CheckScheduler

- `AutoConfig` continues to emit normal schedule/unschedule config events.
- `CheckScheduler` owns deriving all collector check instances from those config events.
- There is no separate metriclookback AutoConfig scheduler in V2.
- Unschedule for one source config must stop both the normal check ID and any derived shadow check ID.

### CheckScheduler To Metriclookback Shadow Policy

- `CheckScheduler` asks metriclookback policy whether a config instance should get a shadow copy.
- Metriclookback policy reads metriclookback config only; it does not load, schedule, run, or stop checks.
- Policy supports default-loader configs and Python checks. It must not use a Go/core-only resolver gate.

### Shadow-Aware Config Metadata

- Shadow instance config is copied before mutation.
- The copied shadow instance receives an internal namespaced execution marker, for example `_datadog.execution_mode: shadow`.
- Normal config bytes and normal instance config bytes are never mutated.
- The marker is available before the second loader/configure call, so both Go and Python checks can opt into shadow-specific behavior.
- Checks are not required to use this key.

### CheckScheduler To CheckLoader

- Normal checks use the normal loader path and normal sender manager.
- Shadow checks use the same selected loader path a second time with the shadow sender manager interface.
- Shadow loading failure must not prevent normal check scheduling.
- The second loaded check is wrapped as `check.ShadowCheck`.

### ShadowCheck Wrapper

- `check.ShadowCheck` wraps the loaded check and preserves inner check behavior.
- It overrides ID to `<source-check-id>:shadow`.
- It overrides interval to the configured shadow interval. V2 default is `1s` whenever metric lookback shadowing is enabled.
- It exposes shadow identity through wrapper/type assertion or helper without changing the base `check.Check` interface.
- Code must not identify shadow checks by parsing the ID suffix except at display/debug boundaries.

### Scheduler Job Dispatch

- One scheduler owns timing for both normal and shadow checks.
- `pkg/collector/scheduler/job.go` owns the route branch for due checks.
- Normal due checks are sent to the runner normal channel.
- Shadow due checks are sent to the runner shadow channel.
- There is no standalone dispatcher component.
- A shared post-scheduler blocking channel for normal and shadow checks is not allowed.

### Runner And Worker Lanes

- Follow Maxime's model from `DataDog/datadog-agent#52726`: one `runner.Runner` component with normal and shadow channels/workers.
- Normal workers read only from the normal channel.
- Shadow workers read only from the shadow channel.
- Shadow worker count does not increase normal worker count.
- Shadow queue/worker saturation must not block normal queue/worker progress.
- Long-running check behavior on the normal path remains unchanged.

### Status And Health

- Normal check status, health, and backend `datadog.agent.check_status` behavior remain unchanged.
- Shadow runs should have local Agent status visibility with `:shadow` IDs.
- Shadow runs must not emit normal backend check-status service checks.
- Runner aggregate counts that are inherently shared must be documented if status cannot fully split them.

### Shadow Sender Interface

- Shadow checks never use the normal backend sender path for metric samples.
- Shadow checks use the metriclookback lookback sender manager supplied by metriclookback wiring.
- The sender manager implements the existing `sender.SenderManager` interface.
- The shadow sender manager is shared across shadow checks and creates/destroys per-check senders by check ID.
- `pkg/collector/check` may refer to the generic sender manager contract, but must not depend on metriclookback storage, ringbuffer, retention, trigger, dump, or flush types.
- The lookback sender writes supported metric samples through a narrow writer interface. The writer implementation may be a fake/noop until the storage workstream lands.
- The shadow pipeline depends only on that writer interface, not on ringbuffer, retention, trigger, dump, or flush internals.
- Unsupported non-metric payload handling belongs to the shadow sender layer before storage; storage should receive supported metric samples only.

### Sender Cleanup And Storage Boundary

- On shadow stop/unschedule, the shadow pipeline releases sender resources for that shadow check.
- Cleanup must not request immediate deletion of retained samples.
- Destroying a shadow sender releases sender resources only. Retained samples remain owned by storage retention.
- Retention, flush, dump, serializer integration, and storage telemetry are owned by the parallel storage/flush workstream.
- Until storage/flush lands, shadow pipeline tests must use the real lookback sender manager with a fake/noop writer implementation.

### Cleanup Ownership

Cleanup must stay split by ownership so normal check behavior is not changed accidentally.

| Cleanup | Owner | Existing or planned hook | Contract |
|---|---|---|---|
| Config-to-check mapping | `CheckScheduler` | `CheckScheduler.Unschedule` and `configToChecks[digest]` | One source config unschedule stops normal and derived shadow IDs. |
| Scheduler timing entry | Scheduler | `scheduler.Scheduler.Cancel(id)` via `collectorImpl.StopCheck` | Removes the ID from scheduler queues before check cancellation. |
| Running check stop | Collector/runner | `collectorImpl.StopCheck` -> `runner.StopCheck(id)` -> `check.Stop()` | Stops currently running normal or shadow check through the correct runner lane. |
| Check cancellation | Collector | `collectorImpl.cancelCheck` -> `check.Cancel()` | Cancels normal or shadow check with bounded timeout. |
| Collector registry | Collector | `collectorImpl.delete(id)` | Removes normal or shadow ID from collector registry and emits stop event. |
| Local check stats | Collector | `expvars.RemoveCheckStats(id)` | Removes local run stats for stopped normal/shadow check unless status design later preserves shadow status. |
| Normal sender cleanup | Middleware | `middleware.CheckWrapper.Cancel()` -> normal `senderManager.DestroySender(id)` | Preserve current normal sender cleanup behavior. |
| Shadow sender cleanup | Shadow wrapper + middleware | `check.ShadowCheck` carries the shadow sender manager used during shadow load; `middleware.NewCheckWrapper` uses that manager for shadow checks; `middleware.CheckWrapper.Cancel()` keeps calling `senderManager.DestroySender(id)` | Same cleanup path for Go and Python shadow checks. Must call shadow sender manager cleanup for shadow IDs, not the normal demux sender manager. Must not delete retained storage samples. |
| Runner shutdown | Runner | `runner.Runner.Stop()` | Must close both normal and shadow channels and stop both worker lanes when Maxime-style runner lanes are implemented. |
| Storage retention cleanup | Parallel storage/flush workstream | storage-owned retention expiry | Not triggered by shadow unschedule; retained samples expire by storage policy. |

### Python Runtime Caveat

- Python checks are eligible when metriclookback policy selects them.
- V2 provides pipeline isolation: separate check instance, shadow config marker, separate sender path, and separate runner channel/workers.
- V2 does not provide Python runtime isolation. Python checks still share the embedded interpreter, GIL, and module-level state.

## Evidence From Current Code

- `pkg/collector/scheduler.go`: `CheckScheduler.Schedule`, `GetChecksFromConfigs`, and `Unschedule` already own AutoConfig check lifecycle and map one config digest to one or more check IDs.
- `comp/collector/collector/impl/collector.go`: `collectorImpl.start` currently creates one `runner.Runner` and one `scheduler.Scheduler`; `RunCheck` registers checks and sends them through the scheduler; `StopCheck` cancels the schedule and stops the runner state.
- `cmd/agent/subcommands/run/metriclookback.go`: current qbranch wiring registers a separate `metric_lookback` AutoConfig scheduler and constructs `metriclookback.NewShadowScheduler`.
- `pkg/collector/metriclookback/scheduler.go`: current qbranch shadow scheduler owns separate load, timing, runner, lifecycle, and stop behavior. V2 should delete or replace this path instead of keeping it alongside shared scheduling.
- `pkg/collector/metriclookback/config.go`: current qbranch already has useful config selection pieces: `Options`, `ShadowConfig`, `DeriveShadowConfigs`, `ShadowIDSuffix`, source ID, shadow ID, and instance-level enable/disable parsing.
- `pkg/collector/metriclookback/lookbacksender` and `pkg/collector/metriclookback/ringbuffer`: current qbranch output/storage pieces are relevant integration points, but storage/flush implementation is a separate workstream from this shadow pipeline plan.
- `metriclookback-python-local-experiments.md`: local experiments showed two Python check instances can run but share module-level state; with `check_runners: 1`, a 3s Python check delayed unrelated checks. This supports pipeline isolation and deployment testing for Python checks, not Python runtime isolation.
- `DataDog/datadog-agent#52726`: Maxime's draft introduces the preferred shape for execution isolation: `check.ShadowCheck`, scheduler job dispatch by shadow type assertion, and one runner component with a normal channel/workers plus dedicated shadow channel/workers. It is intentionally minimal, so this plan expands it with real policy, sender routing, config metadata, lifecycle cleanup, and tests.

## Stack Strategy

Use Graphite to keep each PR scoped. Branch names should use `ella/`.

The Graphite stack should be created off `main`. Pull code from `qbranch-1hz` only when it is still the right implementation shape; otherwise treat it as prior art.

Before cutting the stack:

1. Reconcile prior work against the latest V2 design:
   - inspect `qbranch-1hz` commits/PRs;
   - revisit the current metriclookback plan and diagrams;
   - classify existing work as reusable as-is, reusable only as prior art, or obsolete V1 design to delete/avoid;
   - confirm the stack matches the latest decisions:
     - shared scheduler;
     - scheduler job dispatch in `pkg/collector/scheduler/job.go`;
     - one runner component with normal and shadow channels/workers;
     - cross-language shadow config metadata;
     - Python supported without a Python-specific allowlist;
     - storage/flush owned by the parallel workstream.
2. Confirm which storage/flush pieces are landing separately so this stack depends only on the lookback sender manager plus the agreed writer boundary.
3. Update `metriclookback4.excalidraw` to match this plan:
   - rename `checkDispatcher` to `Scheduler job dispatch (pkg/collector/scheduler/job.go)`;
   - remove `shadowCheckRoute`;
   - replace `Runner [0...1] dedicated shadow runner` with `Runner [1] with normal + shadow channels/workers`;
   - replace cleanup callback wording with `CheckWrapper.Cancel -> shadow SenderManager.DestroySender(id)`;
   - replace `go only` shadow mode wording with cross-language `_datadog.execution_mode: shadow`;
   - mark ringbuffer/storage/flush boxes as the parallel storage workstream.

Recommended order for this shadow-pipeline stack:

1. `ella/mlb-v2-shadow-check`
2. `ella/mlb-v2-shadow-execution-route`
3. `ella/mlb-v2-shadow-config-mode`
4. `ella/mlb-v2-shadow-policy`
5. `ella/mlb-v2-lookback-sender`
6. `ella/mlb-v2-shared-check-loading`
7. `ella/mlb-v2-status-telemetry-docs`

Review strategy:

- Open the larger Agent Runtimes PRs early because they need the longest review cycle.
- `ella/mlb-v2-shadow-execution-route` should stack directly after `ella/mlb-v2-shadow-check`; it only needs the generic shadow marker and does not need shadow loading to exist yet.
- Keep `ella/mlb-v2-shared-check-loading` after the sender manager and execution route. It includes the sender lifecycle hook so reviewers see the hook at the same time as its first real call site, and no intermediate branch loads shadow checks without a correct route and cleanup path.

Parallel storage/flush stack, owned separately:

- lookback writer/storage;
- ringbuffer retention;
- trigger/dump/read path;
- backend flush/serializer integration;
- storage-specific telemetry.

The two stacks should meet at a writer boundary: the shadow pipeline owns the lookback sender manager/sender, and storage owns the writer implementation that retains, flushes, or serializes samples.

## Parallel Workstream: Lookback Storage And Flush

Owner: separate implementer.

Not in this stack:

- `pkg/collector/metriclookback/ringbuffer`
- `pkg/collector/metriclookback/retention.go`
- trigger/dump/read path
- backend flush/serializer integration
- storage-specific telemetry semantics

Contract needed by this stack:

- a writer interface that the shadow sender can call when committing samples;
- sender cleanup through the existing `sender.SenderManager.DestroySender(id)` pattern;
- guarantee that cleanup of a stopped shadow check does not immediately delete retained samples;
- test fake/noop writer implementation so shadow pipeline tests do not depend on storage internals.

Compatibility requirement:

- Until the storage/flush stack lands, this shadow stack should compile and test against a fake/noop writer.
- Once storage lands, wiring can replace the fake writer with the real storage writer without changing sender manager, scheduler job dispatch, or runner-lane behavior.

## Task 1: Add Generic Shadow Check Identity

Status: new or adapted from Maxime-style wrapper direction.

Diagram mapping: `Check Loading -> Shadow Check wrapper (pkg/collector/check)`.

Scope:

- Add a generic shadow wrapper under `pkg/collector/check`.
- The wrapper should:
  - wrap a loaded `check.Check`;
  - override ID to `<source-check-id>:shadow`;
  - override interval to the configured shadow interval, default `1s` once metric lookback shadowing is enabled;
  - expose shadow identity through an optional marker/type assertion;
  - avoid changing the base `check.Check` interface.
- Add helper APIs such as `check.IsShadow(c check.Check)` or equivalent wrapper type assertion.
- Preserve the inner check behavior unless an explicit shadow-mode signal is injected before load/configure.

Shadow-mode config metadata is handled in Task 3. This task only introduces the wrapper/marker used by Agent plumbing.

Tests:

- Wrapper preserves inner check methods except ID/interval/marker.
- `check.IsShadow` returns true only for wrapped checks.
- Shadow wrapper does not require changes to unrelated check implementations.

Validation:

```bash
dda inv test --targets=./pkg/collector/check/...
```

## Task 2: Add Shadow Execution Route In Scheduler Job Dispatch

Status: Agent Runtimes owned surface; keep narrow.

Diagram mapping: `Shared Scheduling -> Scheduler [1]`, `Shared Scheduling -> Scheduler job dispatch`, and `Execution Isolation -> Runner normal/shadow channels + workers`.

Scope:

- Follow Maxime's model from `DataDog/datadog-agent#52726`.
- Edit `pkg/collector/scheduler/scheduler.go`, `pkg/collector/scheduler/job.go`, `pkg/collector/runner/runner.go`, `pkg/collector/worker/worker.go`, and `comp/collector/collector/impl/collector.go`.
- Keep one `scheduler.Scheduler`.
- Keep one `runner.Runner` component instance.
- Add a dedicated shadow input channel on the runner, similar to Maxime's `GetShadowChan()` draft.
- Add dedicated shadow workers that read only from the shadow channel.
- Route checks into scheduler job queues by optional shadow marker/type assertion:
  - normal checks -> runner normal channel;
  - shadow checks -> runner shadow channel.
- Keep separate normal and shadow job queues inside the same `scheduler.Scheduler` instance so a blocked shadow runner channel cannot block the normal queue loop.
- Keep the new concept narrow: runner owns separate worker lanes, scheduler job dispatch owns route selection, and checks identify shadow mode through the wrapper.
- Do not add a standalone dispatcher component. The route branch should live where due checks are already sent to runner pipes: `pkg/collector/scheduler/job.go`.
- Do not create a second scheduler.
- Do not create a separate `runner.Runner` instance unless Agent Runtimes rejects the Maxime-style model.
- Use noop status emission for shadow workers so shadow runs do not emit backend `datadog.agent.check_status`.
- Keep worker execution logic reused where possible. Prefer runner wiring/options that instantiate a separate shadow worker pool over adding shadow-specific execution logic inside `worker.Run`.
- `StopCheck(id)` should remain ID-based and lane-agnostic if possible. If lane-specific state is added, it must stop/remove the check from the correct normal or shadow lane.

Behavior:

- The normal scheduling timeline is shared.
- Shadow backpressure is isolated after scheduler job dispatch.
- Shadow runner worker count must not increase normal runner worker count.
- Normal long-running check behavior must remain unchanged.
- Python checks still share the embedded interpreter, so this is pipeline isolation, not runtime isolation.
- If all normal workers are busy, normal scheduling can still experience normal runner backpressure as it does today.
- If all shadow workers are busy, only shadow scheduling should back up/drop according to shadow policy.
- A shared post-scheduler channel for both normal and shadow checks is not acceptable because it would let a blocked shadow lane block normal checks.
- Normal dispatch must never wait on shadow channel capacity.

Tests:

- Normal checks still run on the normal runner.
- Shadow checks run on the runner's shadow channel/workers.
- Blocking shadow channel/workers does not block normal channel/workers.
- Filling or blocking the shadow channel does not prevent a normal due check from enqueueing/running through the normal channel.
- `StopCheck` remains correct for both normal and shadow IDs.
- Normal runner worker count ignores shadow-only check count.
- Existing long-running check behavior still passes.

Validation:

```bash
dda inv test --targets=./comp/collector/collector/impl
```

## Task 3: Add Shadow Mode Config Metadata

Status: new; small enough to be its own PR.

Diagram mapping: `Check Loading -> Shadow Check wrapper` and `Check Loading -> checkloader.Loader`.

Prior art:

- `DataDog/datadog-agent#52460` is prior art for Go/core checks that may need shadow-aware construction before `Configure`.
- Keep the lesson: some checks, such as GPU-style checks with construction-time telemetry dependencies, may need shadow-safe dependencies before the check is configured.
- Do not carry forward the V1 shadow-only loader catalog or separate `ShadowScheduler` wiring from that PR.
- Prefer the cross-language config metadata first. Add a Go/core construction hook only if a specific check proves that config-time shadow awareness is too late.

Scope:

- Add a helper that injects an internal namespaced shadow execution key into copied shadow instance config, for example `_datadog.execution_mode: shadow`.
- The helper must clone before mutation and must not alter the normal config or normal instance bytes.
- The key should be present before the second loader/configure call so both Go and Python checks can inspect it if they choose to.
- Do not require checks to use this key. It is an opt-in signal for checks that need shadow-specific behavior.
- Checks must not parse `:shadow` IDs to detect shadow mode.

Why separate:

- This is a cross-language contract and should be easy to review independently.
- It can land before the full scheduling/routing work.
- It gives Python checks the same way to detect shadow mode as Go checks without changing the base `check.Check` interface.

Tests:

- Shadow metadata is added only to shadow instance copies.
- Original config bytes are unchanged.
- Existing instance fields are preserved.
- Existing user config cannot accidentally override the internal shadow mode without an explicit policy decision.

Validation:

```bash
dda inv test --targets=./pkg/collector/metriclookback
```

## Task 4: Add Shadow Policy

Status: refactor shadow policy from `pkg/collector/metriclookback/config.go`.

Diagram mapping: `Autodiscovery -> Metriclookback Shadow Policy (pkg/collector/metriclookback)`.

Prior art:

- `DataDog/datadog-agent#52459` explored default-loader eligibility for V1/qbranch when shadow support was Go/core-only.
- The useful lesson is that default-loader configs must not be skipped just because they lack an explicit `loader: core`.
- Do not carry forward the core-only resolver design now that V2 supports Python checks.
- Specifically avoid adding `check.MetadataLoader`, `checkloader.ResolveEffectiveLoader`, `corechecks.RegisterCheckWithLoaderSupport`, disk/network loader-support hooks, or a core-only `ResolveLoader` gate to metriclookback policy.
- Rewrite any useful tests around V2 behavior: selected default-loader configs are eligible for shadow loading, and normal loader selection decides whether the check is Go, Python, or another supported loader during the second load.

Scope:

- Keep a small `pkg/collector/metriclookback` policy that reads:
  - `metric_lookback.enabled`;
  - `metric_lookback.enabled_checks`;
  - optional per-instance `metric_lookback.enabled`.
- Return shadow candidates for selected config instances.
- Preserve source config digest, instance index, source check ID, and desired shadow check ID.
- Clone config and instance data before mutation.
- Include Python checks when metric lookback is enabled; do not add a Python-specific allowlist in this stack.
- Document that broad Python eligibility is for deployment testing and should not be described as full Python runtime isolation.
- Do not include lookback sender, ringbuffer, retention, trigger, dump, or flush behavior here.

Changes from qbranch V1:

- Remove the V1 assumption that only the core Go loader can be shadowed.
- Do not own scheduling, ticking, runner creation, or check stop logic here.
- Do not create a separate AutoConfig scheduler.

Tests:

- Global enablement selects configured checks.
- Per-instance disable wins.
- Non-check and metrics-filtered configs are skipped.
- Source config and instance bytes are not mutated.
- Python candidates are included when normal metriclookback policy selects them.

Validation:

```bash
dda inv test --targets=./pkg/collector/metriclookback
```

## Task 5: Add Lookback Sender Manager

Status: bring over the lookback sender manager/sender from `qbranch-1hz` as its own PR-sized boundary.

Diagram mapping: `Metric Lookback Path -> Sender Manager` and the boundary between `Metric Lookback Path` and the parallel `Local Lookback Storage` workstream.

Prior art:

- `qbranch-1hz` already has `pkg/collector/metriclookback/lookbacksender`.
- That sender manager/sender is part of this shadow-pipeline stack, not the parallel storage/flush stack.
- Reuse it if it still matches the V2 sender contract.

Scope:

- Add or reuse `pkg/collector/metriclookback/lookbacksender`.
- Implement the existing `sender.SenderManager` interface.
- Create and destroy one sender per shadow check ID.
- Capture supported scalar metric samples.
- Drop unsupported payloads before storage with telemetry.
- Call a narrow writer interface on `Commit`.
- Provide a fake/noop writer for tests while storage/flush is still a parallel workstream.
- Do not include ringbuffer, retention, trigger, dump, backend flush, or serializer behavior here.

Ownership line:

- This PR owns sender construction, sender lifecycle methods, sample capture, unsupported payload handling, and the call to the writer boundary.
- The parallel storage/flush stack owns the concrete writer implementation and everything downstream of that writer call.

Tests:

- Sender manager creates and reuses per-check senders.
- `DestroySender(id)` releases the per-check sender.
- Supported scalar metric samples are captured and appended through the writer on `Commit`.
- Unsupported payloads are dropped with telemetry before storage.
- Fake/noop writer tests prove this package does not depend on ringbuffer or flush code.

Validation:

```bash
dda inv test --targets=./pkg/collector/metriclookback/lookbacksender
```

## Task 6: Move Shadow Loading Into CheckScheduler

Status: main V2 lifecycle change.

Diagram mapping: `Autodiscovery -> InitCheckScheduler (pkg/collector)` and `Check Loading -> checkloader.Loader`.

Scope:

- Edit `pkg/collector/scheduler.go`.
- This PR must stack after Task 2 and Task 5 so it has the shadow execution lane and lookback sender manager before loading shadow checks.
- `CheckScheduler.GetChecksFromConfigs` should still load normal checks through normal loader behavior.
- For each config instance selected by the metriclookback shadow policy:
  - make a copied shadow instance config;
  - inject optional internal shadow execution metadata if needed;
  - call the same selected loader path a second time with the shadow sender manager interface;
  - wrap the result in `check.ShadowCheck` with the shadow sender manager override;
  - append the shadow check beside the normal check for `coll.RunCheck`.
- Keep `configToChecks[digest]` as the owner of both normal and shadow IDs so `Unschedule` stops both.
- Normal load failures must not be hidden by shadow load failures.
- Shadow load failures should be recorded/logged but should not prevent normal check scheduling.
- Provide the shared lookback sender manager for shadow loads.
- Keep the sender's writer interface narrow enough that storage implementation can live in the parallel stack.
- Use the existing sender-manager cleanup pattern for both Go and Python shadow checks:
  - `check.ShadowCheck` exposes the shadow sender manager used during shadow load through a generic helper or optional interface, for example `check.SenderManagerOverride(c) (sender.SenderManager, bool)`;
  - `middleware.NewCheckWrapper` uses that override when present and otherwise stores the normal collector sender manager;
  - `middleware.CheckWrapper.Cancel()` continues to call `senderManager.DestroySender(id)`;
  - normal checks keep using the collector's normal demux sender manager.
- On shadow check stop/unschedule, destroy/cancel sender resources for that shadow check, but do not request immediate deletion of retained samples.
- Do not add a separate Go-only or Python-only cleanup path.
- Do not rely on the normal demux sender manager for shadow sender cleanup.
- Do not expose ringbuffer, retention, trigger, dump, or flush types through `pkg/collector/check`.

Reuse:

- Normal `checkloader.Loader` selection semantics.
- Existing `CheckScheduler.Schedule` and `Unschedule` lifecycle.

Delete or stop using:

- `pkg/collector/metriclookback/autoconfig_adapter.go`.
- `metriclookback.NewShadowScheduler` wiring from `cmd/agent/subcommands/run/metriclookback.go`.
- Dedicated shadow ticker/lifecycle from `pkg/collector/metriclookback/scheduler.go`.

Tests:

- Normal-only config produces normal checks only.
- Enabled config produces normal plus shadow checks.
- Shadow loading is enabled only after the scheduler/runner shadow lane exists in the stack.
- Unschedule stops both IDs for the same config digest.
- Sender cleanup is called on shadow stop for both Go and Python shadow checks.
- Shadow sender cleanup uses the generic sender manager override exposed by `check.ShadowCheck`, not the normal demux sender manager.
- Cleanup does not call any "delete retained samples" API.
- Normal sender manager is not used for shadow metric capture.
- Shadow pipeline tests use the real lookback sender manager with a fake/noop writer; they do not require ringbuffer or flush code.
- Shadow load error does not prevent normal check scheduling.
- Shadow config mutation does not alter the original normal instance.

Validation:

```bash
dda inv test --targets=./pkg/collector
```

## Task 7: Status, Telemetry, Docs, And Manual Validation

Status: final integration hardening.

Diagram mapping: cross-cutting validation for `Collector`, `Shared Scheduling`, `Execution Isolation`, `Normal Backend Path`, and `Metric Lookback Path`.

Scope:

- Ensure Agent status separates or clearly labels shadow checks.
- Ensure normal status/health behavior remains unchanged.
- Suppress backend `datadog.agent.check_status` service checks for shadow runs if shadow status should be local-only.
- Local shadow stats should follow normal check behavior. If normal local check stats are removed on unschedule via `expvars.RemoveCheckStats(id)`, shadow local stats should also be removed on unschedule.
- Register shadow-pipeline telemetry only for scheduler/routing/sender concerns, including unsupported payload drops in the lookback sender.
- Leave storage, writer append, retention, dump, and flush telemetry to the parallel storage workstream.
- Update PR description and architecture docs with V1 vs V2 diagrams.
- Update config docs or comments for:
  - `metric_lookback.enabled`;
  - `metric_lookback.enabled_checks`;
  - shadow interval default `1s` once enabled;
  - Python support caveat;
  - `_datadog.execution_mode: shadow` or whichever internal metadata key is selected.

Manual validation:

- Run `cpu` and `ntp` with metric lookback enabled.
- Verify normal check status still exists.
- Verify shadow status rows use `:shadow` IDs.
- If the storage/flush stack is present, verify lookback telemetry increments:

```bash
./bin/agent/agent diagnose show-metadata agent-full-telemetry -c dev/dist/datadog.yaml | grep -i metric_lookback
```

- Repeat synthetic Python experiment when Python shadow checks are enabled for deployment validation.
- If the storage/flush stack is not present yet, use the real lookback sender manager with a fake/noop writer and validate routing/lifecycle only.

Validation:

```bash
dda inv agent.build --build-exclude=systemd
dda inv test --targets=./pkg/collector ./comp/collector/collector/impl ./pkg/collector/metriclookback/...
```

## Legacy Code To Remove Or Avoid

Remove or stop using these V1-specific pieces in the V2 stack:

- `metriclookback.NewAutoConfigShadowAdapter`
- `metriclookback.ShadowScheduler`
- `ShadowRunnerFactory` and dedicated shadow ticker ownership in `pkg/collector/metriclookback/scheduler.go`
- `registerMetricLookbackScheduler` registering a separate AutoConfig scheduler
- Shadow-only loader catalog replacement that bypasses normal selected-loader semantics
- Any standalone dispatcher component outside scheduler job dispatch

Keep or adapt these pieces:

- Shadow config selection logic from `pkg/collector/metriclookback/config.go`
- Lookback sender manager interface
- Sender cleanup hook
- Unsupported payload drop telemetry in the lookback sender
- Any test fake/noop writer needed to keep the shadow pipeline stack independent

Leave to the parallel storage/flush workstream:

- Ringbuffer/retention
- Trigger/dump/read path
- Backend flush/serializer integration
- Telemetry counters/gauges for storage, writer append, retention, dump, and flush

## Resolved Design Choices

- Shadow-aware check signal: use an internal namespaced config key such as `_datadog.execution_mode: shadow`, injected only into copied shadow instance config before the second load. This is implemented as its own PR, `ella/mlb-v2-shadow-config-mode`.
- Python enablement: do not add a Python-specific allowlist for now. Document that Python support is pipeline isolation only because Python checks still share the embedded interpreter, GIL, and module-level state.
- Execution isolation: follow Maxime's model from `DataDog/datadog-agent#52726`: one `runner.Runner` component with separate normal and shadow channels/workers. Scheduler job dispatch in `pkg/collector/scheduler/job.go` routes due checks to the correct channel. A second runner instance is only a fallback if Agent Runtimes rejects this model.
- Sender ownership: the shadow pipeline owns the lookback `sender.SenderManager`/sender layer, reusing `pkg/collector/metriclookback/lookbacksender` from `qbranch-1hz` if it still fits V2. The parallel storage/flush workstream owns the writer implementation and everything downstream.
- Backpressure acceptance criterion: fill or block the shadow channel/worker path and prove a normal due check can still enqueue/run through the normal runner channel/workers.
- Local stats behavior: shadow checks follow the normal check lifecycle. If normal stats are removed on unschedule, shadow stats are removed on unschedule too.
- Shadow interval: default to `1s` once metric lookback shadowing is enabled.
- Stack base: build the Graphite stack off `main`; use `qbranch-1hz` only for reusable commits or prior art.

## Review Ownership Map

| Area | Code | Proposed status | Stakeholder |
|---|---|---|---|
| Autodiscovery events | `comp/core/autodiscovery` | unchanged | Agent Runtimes |
| CheckScheduler lifecycle | `pkg/collector/scheduler.go` | edited | Agent Runtimes |
| Collector wiring | `comp/collector/collector/impl` | edited; passes normal and shadow runner channels into scheduler | Agent Runtimes |
| Scheduler job dispatch | `pkg/collector/scheduler/job.go` | edited; routes due checks by shadow marker to normal vs shadow runner channel | Agent Runtimes |
| Runner/worker execution | `pkg/collector/runner`, `pkg/collector/worker` | edited; one runner with normal + shadow channels/workers | Agent Runtimes |
| Check identity wrapper | `pkg/collector/check` | new | Agent Runtimes |
| Metriclookback shadow policy | `pkg/collector/metriclookback` | refactored/new | Agent Metric Pipelines |
| Shadow sender interface/hooks | `pkg/collector/metriclookback`, `cmd/agent/subcommands/run/metriclookback.go` | new/edited | Agent Metric Pipelines |
| Lookback storage/flush | `pkg/collector/metriclookback/ringbuffer`, `retention.go`, trigger/dump/serializer wiring | parallel workstream | Agent Metric Pipelines |
| Agent run wiring | `cmd/agent/subcommands/run/metriclookback.go` | edited | Agent Platform / Agent Metric Pipelines |

## Done Criteria

- One shared scheduler owns timing for normal and shadow checks.
- Normal and shadow checks are loaded from the same AutoConfig event lifecycle.
- Shadow checks use copied configs and do not mutate normal configs.
- Shadow checks are identifiable without changing the base `check.Check` interface.
- Shadow execution is routed to an isolated lane by scheduler job dispatch.
- Blocking shadow execution does not block normal runner input.
- Normal backend sender behavior is unchanged.
- Shadow metrics go to the shadow sender interface only, never normal backend sender.
- Shadow sender cleanup happens on stop/unschedule without requesting early deletion of retained samples.
- The shadow pipeline can be tested without the storage/flush implementation.
- Tests cover lifecycle, routing, backpressure isolation, and sender cleanup.
- PR descriptions include clear V1 vs V2 diagrams and Python isolation caveat.
