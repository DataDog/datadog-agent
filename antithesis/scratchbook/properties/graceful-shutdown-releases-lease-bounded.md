# graceful-shutdown-releases-lease-bounded — Shutdown completes and releases the lease within a bound, even under partition

**Type:** Liveness · **Assertion:** `Always` · **Priority:** P1 · **Intent:** known-defect-reproducer

**Provenance:** merged from 1 discovery agent(s): graceful-shutdown-hang-releaseoncancel

## Property

After SIGTERM the DCA completes shutdown and releases its leader lease within a bounded deadline, even when the apiserver is partitioned, so failover is not delayed the full lease duration.


## Invariant / assertion

`assert.Always` (bounded-time liveness): after SIGTERM under an apiserver partition, the process completes shutdown and the lease is released-or-expires within a bounded deadline. Framed as Always-within-deadline so a HANG in the ReleaseOnCancel network call FAILS the assertion. (Evaluation caught the original Sometimes(released) as an assertion-shape inversion — a success-witness can never fire on 'sometimes it hangs', which is the actual bug.)


## Paired witness (R1 — proves the hazardous window was scheduled)

`Reachable`: SIGTERM was delivered while the leader was partitioned from the apiserver (ReleaseOnCancel attempted under partition).


## Antithesis angle

ReleaseOnCancel (leader_election_release_on_shutdown, default true) makes a network call on ctx-cancel to shorten the lease to 1s (leaderelection_engine.go:196-198). If shutdown coincides with an apiserver partition, that call hangs/fails → the lease is neither released nor renewed → full LeaseDuration (60s) gap with no leader. Partition the leader<->apiserver, then SIGTERM it.


## Why it matters

A shutdown that hangs on the release call blocks the pod's termination and delays failover by up to a full lease duration — during a rolling upgrade this stacks up as cluster-wide leader-gated downtime.


## Mechanism refinement (from open-question investigation)

INVALIDATION of the core mechanism as framed. The premise 'ReleaseOnCancel hangs/blocks shutdown indefinitely under partition' is false for client-go v0.35.5: (a) release() runs on a fresh context.Background() bounded by RenewDeadline (LeaseDuration/2 = 30s default) at leaderelection.go:311-335, so it is time-bounded, not indefinite; (b) the leader-election goroutine is bare (leaderelection.go:199) and NOT awaited by the shutdown wg (command.go:746), so the release cannot block pod termination at all. The residual REAL risk is a different failure: if the process exits before the bounded release completes, the lease is not shortened and the successor waits up to full LeaseDuration (failover gap) — an abandoned-release, not a hang. Recommend re-framing the assertion away from 'shutdown completes within bound' (already guaranteed) toward the failover gap: lease is shortened OR successor still acquires within ~LeaseDuration.


## Fault dependencies

- network partition (leader<->apiserver; enabled by default)
- SIGTERM delivery (ordinary shutdown; node termination graceful mode)
- requires leader_election enabled + >=2 replicas


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

MISSING. `assert.Sometimes` on bounded shutdown completion under partition; the release call should have a timeout — the assertion documents the requirement.


## Open questions (post-investigation)

- Do long-lived gRPC/HTTP streams (tagger/kube-metadata) or an fx OnStop hook prolong process exit? (Largely moot: the release call is bounded and not awaited, so no indefinite hang either way.) Confirmed only that shutdown does NOT wait on the leader-election goroutine (command.go:746 wg covers only extmetrics+admission) and StopServer() is never called; the primary API/gRPC listener drain path on exit was not fully traced. `(partial)`


### Investigation Log

#### Q1: Default of leader_election_release_on_shutdown

Examined pkg/config/setup/common_settings.go:282 and pkg/util/kubernetes/apiserver/leaderelection/leaderelection_engine.go:198. Found BindEnvAndSetDefault("leader_election_release_on_shutdown", true). Concluded: default TRUE, so the ReleaseOnCancel path is active by default. RESOLVED.

#### Q3: Does client-go ReleaseOnCancel inherit the cancelled ctx or use a bounded context?

Examined client-go v0.35.5 (go.mod) tools/leaderelection/leaderelection.go:304-335. Found release() at :311 creates a FRESH context.Background(), NOT the cancelled ctx; renew() calls release() synchronously after the renew loop exits (:305). Concluded: does not inherit the cancelled ctx. RESOLVED.

#### Q4: Does the ReleaseOnCancel call have a timeout, or block indefinitely under partition?

Examined leaderelection.go:312 context.WithTimeout(ctx, le.config.RenewDeadline); both Lock.Get (:315) and Lock.Update (:335) use that timeoutCtx. RenewDeadline = LeaseDuration/2 (leaderelection_engine.go:201), i.e. 30s at the 60s default. Concluded: the release is BOUNDED (max ~RenewDeadline per call), cannot block indefinitely. RESOLVED.

#### Q2: Do long-lived gRPC streams keep the process alive at exit vs listener closing fast?

Examined command.go:724-760 shutdown path. Found mainCtxCancel() then wg.Wait() (:746) covers only external-metrics + admission webhook servers; the leader-election goroutine is launched bare (leaderelection.go:199) and is NOT in wg, and cmd/cluster-agent/api/server.go StopServer() is never called. Did not fully trace whether an fx OnStop hook drains the primary API/gRPC listener and prolongs exit. PARTIAL (and moot given bounded, un-awaited release).


---

## Source discovery evidence (raw, per contributing agent)


### from `graceful-shutdown-hang-releaseoncancel`

## Mechanism (verified)

### No API drain on shutdown
- `cmd/cluster-agent/api/server.go:166` defines `StopServer()` (closes the listener).
- The shutdown path `command.go:725-761` never calls it. It does: read health (`:729`), `mainCtxCancel()` (`:737`), stop kubeactions retriever (`:741`), `wg.Wait()` for external-metrics+admission (`:746`), close `stopCh`/`validatingStopCh` (`:748-751`), and `metricsServer.Shutdown()` (`:753`). The primary muxed HTTPS/gRPC listener (opened at `server.go:145`, served at `server.go:150`) is closed only implicitly at process exit.
- Consequence: in-flight node-agent requests (`/api/v1/clusterchecks/configs/{id}`, tagger/kube-metadata gRPC streams) are not drained.

### ReleaseOnCancel network call on the cancel path
- `leaderelection_engine.go:196-198`: `ReleaseOnCancel: leader_election_release_on_shutdown`; comment: "performs a network call on shutdown" to set LeaseDuration to 1s.
- `leaderelection.go:236-248` `runLeaderElection()` loops calling `le.leaderElector.Run(le.ctx)`. client-go's `Run` executes the ReleaseOnCancel update synchronously when `le.ctx` is cancelled, before returning.
- The engine ctx is the main ctx (`CreateGlobalLeaderEngine(mainCtx)`, command.go:345), cancelled at `command.go:737`.
- This goroutine is launched bare (`go le.runLeaderElection()`, leaderelection.go:199) and is **not** part of the `wg` that shutdown waits on. So: if the process lingers, the release call hangs against the partitioned apiserver; if the process exits promptly, the release is abandoned. Either way the lease is not reliably shortened.

## Failure scenario

1. Leader holds the lease; Antithesis partitions leader↔apiserver.
2. Orchestrator sends SIGTERM (rollout/deploy).
3. `mainCtxCancel()` → ReleaseOnCancel network call blocks (partition).
4. Lease is not shortened to 1s. Successor replica must wait the full `leader_lease_duration` (default 60s, leaderelection_engine.go:200) before acquiring → 60s cluster-check dispatch gap instead of ~1s.
5. Concurrently, node requests in flight against the un-drained API listener are reset.

## Key observations

- The metrics server IS drained (`command.go:753`) but the primary API server is not — asymmetry strongly suggesting the missing `StopServer()` is a bug, not a decision.
- `ReleaseOnCancel` defaults: `leader_election_release_on_shutdown` — verify default; if true, the hang path is active by default.

## Timing window

Failover degradation = up to full `leader_lease_duration` (60s default) when the release call cannot complete.
