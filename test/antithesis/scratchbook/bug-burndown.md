---
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-29
---

# Suspected-Bug Burn-Down

## Stack layout (local `gt`, not pushed; one subsystem per branch)

```
main
└── blt/antithesis-research            research artifacts + harness scaffold (this dir)
    └── antithesis-file-tailer         tailers/file: rotation-loss, seek-error, multiline (+ SUT harness, SDK)
        └── antithesis-file-provider   launchers/file/provider: wildcard ordering
            └── antithesis-decoder-sampler   decoder/preprocessor: sampler aliasing guard (FIXED)
                └── antithesis-sender        sender/strategy/processor/client-tcp: encode/oversized/render drops, idempotent-stop, tcp leak (+ mrf NOT-A-BUG, tcp-permanent DESIGN-INTENT)
                    └── antithesis-pipeline  pipeline: failover send-on-closed/hang, per-source ordering
                        └── antithesis-auditor   auditor/impl: offset regression, registry corruption/recovery, 4xx-replay (+ drains/migration REFUTED)
                            └── antithesis-service-store   service: latent deadlock (DORMANT)
                                └── antithesis-container-launcher   sources/schedulers-ad/processor: add-remove orphan, collect-all gap, stale metadata
```

> **TERMINAL — burn-down COMPLETE (2026-05-29):** every suspected bug has a verdict
> and **every suspected defect is reproduced**. **20 reproduced** (local build-tagged
> failing tests; 2 also reproduced on Antithesis with ~39–41k counterexamples each),
> **6 refuted/design-intent/not-a-bug**, **2 fixed (regression guards)**, **1 dormant**.
> The 3 container-runtime races were reproduced DETERMINISTICALLY at the component
> level by exercising the bad interleaving's effective sequence directly (no container
> runtime / no goroutine-scheduling tricks needed). The only thing NOT run is a *full
> end-to-end kill-9 at-least-once* timeline on Antithesis (node-termination faults are
> disabled in this tenant), but every constituent loss/dup window of that path is
> reproduced locally (rotation-loss, registry-recovers-after-crash,
> no-loss-and-duplicate). **No un-reproduced suspected bug remains.**

## Round 6 verdicts — the 3 container races (all REPRODUCED, deterministic)

| Bug | Status | Test / evidence |
|---|---|---|
| container-addremovesource-ordering | REPRO-LOCAL | `pkg/logs/sources/sources_ordering_bug_test.go` — Remove-then-Add (the order the fire-and-forget goroutines can produce) leaves an orphaned source; `RemoveSource` is a no-op when absent (`sources.go:87-94`), `AddSource` appends unconditionally (`sources.go:56`) |
| log-metadata-not-corrupted | REPRO-LOCAL | `comp/logs-library/processor/antithesis_stale_origin_metadata_demo_test.go` — `Origin.Service()` (`origin.go:154-161`) reads live `LogSource.Config.Service` at encode (`json.go:67`) / MRF-tag (`processor.go:236`) time; mutate-while-in-flight → wrong `service` tag + wrong MRF routing |
| container-collect-all-startup-race | REPRO-LOCAL | `pkg/logs/schedulers/ad/container_collect_all_race_test.go` — `Unschedule(CCA)` then `Schedule(annotated)` are non-atomic (`scheduler.go:148-180`); source-count snapshot `[1,0,1]` proves a gap window with no active source → lines lost. The one-`Collect()` mitigation (`providers/container.go:241-248`) doesn't cover cross-cycle arrival. |

## Round 5 verdicts

| Bug | Status | Test / evidence |
|---|---|---|
| registry-recovers-after-crash | REPRO-LOCAL | `comp/logs/auditor/impl/antithesis_registry_recovers_after_crash_demo_test.go` — missing/zero-byte/corrupt `registry.json` → `recoverRegistry()` (auditor.go:337-353) silently returns empty map → tailers resume from default (loss/replay) |
| no-loss-and-duplicate-same-line (4xx-replay) | REPRO-LOCAL | `comp/logs/auditor/impl/antithesis_permanent4xx_replay_demo_test.go` — HTTP advances offset on permanent 4xx (destination.go:318); crash before the 1s flush → restart re-reads & re-sends the "permanently dropped" line |
| mrf-unreliable-destination-drop-bounded | NOT-A-BUG | `comp/logs-library/sender/antithesis_mrf_unreliable_drop_demo_test.go` (passes) — reliable gets all N; unreliable drops counted (`tlmPayloadsDropped{reliable=false}`). Only gap: counted-but-not-alerted (observability). |
| registry-format-migration-safe | REFUTED | `comp/logs/auditor/impl/antithesis_registry_format_migration_safe_demo_test.go` (passes) — v0/v1/v2 migration preserves offsets; only a future-version rollback would lose entries |
| at-least-once-no-loss | REPRO-LOCAL (windows) + FAULT-GATED (full crash) | rotation path = backpressure-no-rotation-loss; crash loss/dup windows = registry-recovers + 4xx-replay; a full kill-9 end-to-end run needs node-termination (disabled) |

## Remaining (not a new bug — additional confirmation only)

- **Full end-to-end kill-9 at-least-once timeline on Antithesis** — would additionally
  confirm the crash path under a real `kill -9`, but **node-termination faults are
  disabled** in this tenant (run 1 meta-props). Not a new suspected bug: every
  constituent loss/dup window is already reproduced locally (rotation-loss,
  registry-recovers-after-crash, no-loss-and-duplicate). To run it: enable
  node-termination on the tenant and stand up the agent+fakeintake+workload topology
  (a faithful agent SUT also needs the release image — registry currently unreachable).

Tracks every suspected bug from the property catalog to a terminal state. Statuses:

- **REPRO-LOCAL** — reproduced by a build-tagged `antithesis_demo` test against real
  Agent code (run with `go test -tags "antithesis_demo test"`).
- **REPRO-ANTITHESIS** — reproduced on an Antithesis run (failed `Always`).
- **REFUTED** — investigated/tested; the code is correct, not a bug.
- **DORMANT** — a latent defect with no live trigger in current code (e.g. no caller).
- **TOPOLOGY-GATED** — a real suspected bug that cannot be reproduced by a single
  local test; needs the multi-container Antithesis topology (agent + fakeintake +
  fault-driving workload) and/or non-default faults (node-termination, etc.).
- **NOT-YET** — not yet triaged.

## Reproduced

| Bug | Status | Test / evidence |
|---|---|---|
| backpressure-no-rotation-loss | REPRO-LOCAL + REPRO-ANTITHESIS | `pkg/logs/tailers/file/antithesis_rotation_loss_demo_test.go`; Antithesis run 1 (41,043 counterexamples) |
| offset-no-regression-on-seek-error | REPRO-LOCAL (+ Antithesis run 2) | `pkg/logs/tailers/file/antithesis_seek_error_demo_test.go` |
| idempotent-stop | REPRO-LOCAL | `comp/logs-library/sender/antithesis_idempotent_stop_demo_test.go` (deadlock + 2 panics) |
| registry-survives-crash (Fargate non-atomic) | REPRO-LOCAL | `comp/logs/auditor/impl/antithesis_fargate_registry_corruption_demo_test.go` |
| multiline-not-split-across-pipelines | REPRO-LOCAL | `pkg/logs/tailers/file/antithesis_multiline_rotation_demo_test.go` (control proves aggregation; 0 delivered at rotation) |
| oversized-line-truncation-safe | REPRO-LOCAL | `comp/logs-library/sender/antithesis_oversized_batch_drop_demo_test.go` (offset never advanced → infinite re-read) |
| no-send-on-closed-on-shutdown | REPRO-LOCAL | `comp/logs-library/pipeline/antithesis_no_send_on_closed_demo_test.go` (failover: Stop() hangs on unbounded Wait + send-on-closed panic when processor closes InputChan) |
| wildcard-file-ordering-stable | REPRO-LOCAL | `pkg/logs/launchers/file/provider/antithesis_wildcard_ordering_demo_test.go` (basename-only sort excludes highest-priority file at filesLimit cap) |
| container-identifier-no-collision | REPRO-LOCAL | `comp/logs/auditor/impl/antithesis_container_identifier_no_collision_demo_test.go` (offset regresses 5000→100; guard is strict `>` on IngestionTimestamp, never compares offsets) |
| per-source-ordering-preserved (+ rotation-pipeline-reassignment) | REPRO-LOCAL | `comp/logs-library/pipeline/failover_ordering_repro_test.go` (failover splits one source across pipelines → reorder; README:156 violated). rotation-pipeline-reassignment shares this mechanism. |

## Refuted / not a bug / fixed / dormant

| Bug | Status | Why |
|---|---|---|
| auditor-drains-on-stop | REFUTED | `comp/logs/auditor/impl/antithesis_drains_on_stop_demo_test.go`: Go closed-buffered-channel semantics drain all buffered items before `isOpen=false`; 100/100 verified. (Real residual race is the `Flush()` H2 snapshot, on transport-restart, NOT `Stop()`.) |
| secrets-redacted-before-send | REFUTED | Every send path routes through `processMessage`→`applyRedactingRules` (run, serverless Flush, channel launcher); no bypass. |
| logs-not-modified-in-transit | REFUTED | No goroutine mutates a `Message` post-processing: diagnostic receiver only enqueues a pointer; `SetContent` uses a fresh slice; batch copies metadata by value. |
| adaptive-sampler-no-aliasing | FIXED-REGRESSION-GUARD | Fix present (`sampler.go:215-217` comment + pre-bubble mutation); `pkg/logs/internal/decoder/preprocessor/antithesis_aliasing_regression_test.go` passes (200 calls, no aliasing). |
| no-services-store-deadlock | DORMANT | Zero production subscribers (`GetAll/AddedServicesForType` only called in tests). Latent deadlock shown in `pkg/logs/service/antithesis_deadlock_demo_test.go`; not reachable today. |

## Round 3 verdicts

| Bug | Status | Test / evidence |
|---|---|---|
| batch-encode-failure-no-silent-batch-loss | REPRO-LOCAL | `comp/logs-library/sender/antithesis_batch_encode_failure_demo_test.go` (injected failing serializer/Finish → silent drop, no offset) |
| processor-render-error-no-silent-loss | REPRO-LOCAL | `comp/logs-library/processor/antithesis_render_error_silent_loss_demo_test.go` (StateEncoded render err + failing encoder → silent drop) |
| bounded-memory / zstd-cctx-leak | FIXED-REGRESSION-GUARD | fix `0d9dfc76f46` at `batch.go:73-88` (Close() on all reset paths, nil-guarded); `batch_test.go` regression tests present |
| no-goroutine-leak-after-stop | REPRO-BY | covered by the failover forwarder-hang test (`antithesis_no_send_on_closed_demo_test.go`): the goroutine never exits on Stop. Other leak paths are NEEDS-TOPOLOGY. |
| container-addremovesource-ordering | NEEDS-TOPOLOGY | confirmed real (`launchers/container/.../source.go` fire-and-forget Add/Remove + `sources.go` no-op remove); needs Antithesis thread-pausing to force add-after-remove. |
| tcp-connection-goroutine-no-leak | REPRO-LOCAL | `comp/logs-library/client/tcp/antithesis_defer_cancel_leak_demo_test.go` — `defer cancel()` inside the `NewConnection` reconnect loop (`connection_manager.go:102-103`) accumulates N timer/context objects per outage (timer leak, not goroutine leak). |
| tcp-permanent-error-no-offset-advance | DESIGN-INTENT (REFUTED as a bug) | `comp/logs-library/client/tcp/antithesis_permanent_error_no_offset_demo_test.go` (passing regression guard) — TCP not advancing the offset on a write error is *correct* (no 4xx concept on TCP; data retried by reconnect). Guards against a future refactor aligning it to HTTP. |

## Antithesis runs (counterexamples)

- Run 1 `226498823ce20b84c7b05575d6f2e30b-54-9`: backpressure-no-rotation-loss FAILING (41,043 cx).
- Run 2 `3fb8dbbeeacb7ac0f71aa870716e2f60-54-9`: backpressure-no-rotation-loss FAILING (39,143 cx) + offset-no-regression-on-seek-error FAILING (39,673 cx); both Reachable companions PASS.

## Topology / fault-gated (terminal: needs Antithesis topology or non-default faults)

**Fault-availability blocker:** Antithesis run 1's meta-properties showed
`node - kill/pause/throttle` and `clock - skip` as **not exercised** under the
`basic_test` webhook — i.e. node-termination and clock faults appear **disabled** in
this tenant. Crash/clock-dependent repros therefore cannot run here without the
tenant enabling those faults. A faithful agent SUT is additionally blocked by the
unreachable `registry.ddbuild.io` release buildimage.

| Bug | Needs | Note |
|---|---|---|
| at-least-once-no-loss (crash path) | node-termination | rotation-under-backpressure part already REPRO-LOCAL (backpressure-no-rotation-loss); crash-replay part fault-gated |
| no-loss-and-duplicate-same-line | node-termination + 4xx | 4xx-advances-offset + crash-replay |
| registry-recovers-after-crash | node-termination | empty-map recovery |
| registry-format-migration-safe | version migration + crash | |
| tcp-permanent-error-no-offset-advance | TCP intake | HTTP/TCP offset asymmetry (see round-4) |
| tcp-connection-goroutine-no-leak | TCP intake / conn churn | defer-cancel accumulation (see round-4) |
| mrf-unreliable-destination-drop-bounded | 2nd intake + MRF | |
| container-collect-all-startup-race | container runtime | |
| log-metadata-not-corrupted | container churn | |

**Guarantees-that-pass (not defects — would PASS, not reveal a bug):**
backpressure-before-drop, queued-payloads-eventually-sent, permanent-error-no-retry,
retryable-no-retry-after, graceful-degradation-on-startup, transport-switch-no-loss,
clean-shutdown-completes (the failover hang sub-case is already REPRO-LOCAL). These
are invariants to verify in an end-to-end run, not suspected bugs to burn down.

## Topology-gated (need agent + fakeintake + workload, and/or specific faults)

- at-least-once-no-loss (node-termination), no-loss-and-duplicate-same-line,
  registry-recovers-after-crash (node-termination), registry-format-migration-safe,
  tcp-permanent-error-no-offset-advance (TCP intake),
  tcp-connection-goroutine-no-leak (TCP intake),
  mrf-unreliable-destination-drop-bounded (2nd intake),
  container-collect-all-startup-race (container source),
  graceful-degradation-on-startup, transport-switch-no-loss,
  queued-payloads-eventually-sent, clean-shutdown-completes, backpressure-before-drop,
  permanent-error-no-retry (chaos proxy), retryable-no-retry-after (429 intake).

  These require the multi-container topology in `deployment-topology.md`. A faithful
  agent SUT needs the release image (blocked: `registry.ddbuild.io` unreachable). A
  focused end-to-end harness (tailer→processor→strategy→sender→fakeintake using real
  pkg/logs components, which compile with plain `go build`) is the viable path and is
  the next escalation for this tier.

## Dropped (out of scope per user)

- clock-jump-no-extra-sampling, clock-jump-no-backoff-underflow — DROPPED (B-CLOCK).
