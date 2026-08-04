# kubeactions-at-most-once — A KubeAction executes at most once across restarts and failover

**Type:** Safety · **Assertion:** `Always` · **Priority:** P1 · **Intent:** known-defect-reproducer

**Provenance:** merged from 2 discovery agent(s): kubeactions-at-most-once-execution-across-failover, kubeactions-at-most-once-execution-not-durable

## Property

A KubeAction identified by (metadata.ID, metadata.Version) has its mutating executor (delete_pod, restart_deployment, patch_deployment, ...) run at most once against the cluster, even across DCA restarts and leadership handovers.


## Invariant / assertion

`assert.Always(action_executed_at_most_once)`: for each (ID,Version), the executor side effect fires no more than once across the run. Always fits — an at-most-once safety guarantee. NOTE: dedup state (ActionStore) is an in-memory map wiped on restart, so this property is expected to FAIL under a crash between Claim and MarkExecuted, or under two leaders — that is the finding.


## Paired witness (R1 — proves the hazardous window was scheduled)

`Reachable`: two replicas were simultaneously in leader/dispatch state, or a crash landed between Claim and MarkExecuted.


## Antithesis angle

ActionStore tracks processed actions in-memory only (action_store.go). Claim marks StatusClaimed, then the side effect runs, then MarkExecuted. A crash after the mutation but before MarkExecuted, or a restart (empty map) with the action re-delivered within its 1-min TTL, re-executes it. If dispatch-implies-lease-holder is violated (two leaders), each has its own map → guaranteed double execution. Requires node termination to exercise the crash-replay path.


## Why it matters

Double execution of delete_pod / restart_deployment is a destructive, externally-visible action taken twice — a real operational hazard (e.g. two rollout restarts). Ties directly to the one-leader guarantee. Merged from 2 focus agents.


## Mechanism refinement (from open-question investigation)

Refine (no invalidation): the duplicate-execution hazard requires a NEW process (DCA restart / new leader pod), not a mere in-place leadership handover — a follower that already holds the config in its RC client state is NOT re-delivered on promotion (client.go fires listeners only on changedProducts). The assertion/witness should tie the double-execution to a process restart between Claim and the empty-map replay, consistent with the node-termination fault dependency already listed. All other premises (in-memory-only dedup, stable ActionKey, non-idempotent executors) are confirmed by primary source.


## Fault dependencies

- node termination (DISABLED by default — required for the crash-replay variant)
- leader election enabled + >=2 replicas (kubeactions is leader-gated; inert otherwise)
- network partition to induce split brain
- requires remote config client


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

MISSING. `assert.Always` around the executor keyed by (ID,Version) with a durable/observed dedup record; the property argues dedup must survive restart. Confirm ActionStore has no persistence before asserting the failure.


## Open questions (post-investigation)

- Does backend ActionTTL/timestamp practice make the 1-min execution window commonly still-open at failover time in real deployments? Code confirms the only time gate is ValidateTimestamp vs action.Timestamp with ActionTTL=1m; whether the action's creation timestamp is still <1m at failover is a backend delivery-timing practice, not determinable from SUT code. `(needs human input)`


### Investigation Log

#### Q1: Does the RC backend re-push an already-acknowledged K8S_ACTIONS config to a new leader, or only on version change?

Examined client.go update()/applyUpdate (513-547) and SubscribeIgnoreExpiration (393). Found: the RC client is per-process; the listener fires only for changedProducts, and it is handed state.GetConfigs(product) (the FULL active config set), not a delta (client.go:541). A freshly started leader pod = new process = empty client state, so the first successful poll marks every active K8S_ACTIONS config as changed and replays the full set to actionsCallback. Not gated on version change. NOTE: an in-place follower->leader promotion inside an already-running process does NOT re-fire the callback (config already in local state, no changedProducts). Conclusion: RESOLVED — redelivery to a NEW process is a full replay; the duplicate hinges on a new process (restart/new pod), which matches the property's node-termination fault requirement, not a mere leadership handover.

#### Q2: Are any executors truly idempotent (restart_deployment annotation patch may coalesce)?

Examined restart_deployment.go:61 and setup.go:73-77. Found: restart sets annotation kubectl.kubernetes.io/restartedAt = time.Now().Format(RFC3339) on every Execute — a new value each run, so a repeat is a distinct second rollout (NOT idempotent/coalescing). delete_pod/patch_deployment/rollback_deployment are likewise mutating. Only get_resource is read-only (harmless if repeated). Conclusion: RESOLVED — no mutating executor is idempotent; the annotation patch does not coalesce because the timestamp differs.

#### Q3: Is metadata.Version stable across redelivery (if it increments, ActionKey changes and dedup is moot)?

Examined processor.go:90-93 (ActionKey={ID,Version} from rawConfig.Metadata) and state/repository.go:218,447 + repository_test.go (Version stays 1 across redelivery, becomes 2 only on content change). Found: Metadata.Version is the TUF/backend config-file version, incremented only when config content changes, stable across redeliveries of the same config. Conclusion: RESOLVED — ActionKey is stable across redelivery, so dedup is meaningful WITHIN a process; the only failure mode is the empty in-memory map on a fresh process, exactly as the property claims (dedup key is not the problem).

#### Q5: Is there any persistence/leader-scoped fencing for ActionStore beyond the in-memory map?

Examined action_store.go (executed map[string]ActionRecord, sync.RWMutex; NewActionStore makes a fresh map, setup.go:42 calls it once per Setup) and cleanup() (only removes records older than RecordRetentionTTL). Found: no disk/CRD/k8s backing, no cross-replica sharing, no leader-generation/epoch fence. Claim inserts StatusClaimed but is not persisted anywhere durable. Conclusion: RESOLVED — dedup is purely in-memory and non-durable; the property's premise (state wiped on restart, no fencing) is confirmed.

#### Q4: backend ActionTTL window still-open at failover

Examined ValidateTimestamp/isExpired (action_store.go:78-104): the sole time bound is action.Timestamp age vs ActionTTL=1m, plus a 10s future-skew buffer. Whether a redelivered action is still within 1m at failover depends on backend delivery/timestamp practice, which is outside the cluster-agent code. Conclusion: code side confirmed (1-min gate is the only barrier and a prompt failover easily fits inside it), but the real-world commonality is a backend-practice judgment -> kept as needs-human.


---

## Source discovery evidence (raw, per contributing agent)


### from `kubeactions-at-most-once-execution-across-failover`

## Mechanism (verified in source)

**Dedup is in-memory and per-process.** `ActionStore` holds `executed map[string]ActionRecord` (action_store.go:56-60), created fresh in `NewActionStore` (called once from `Setup`, setup.go:42) at each process start. There is no persistence and no cross-replica sharing.

**Claim is the only guard.** `ActionProcessor.Process` calls `p.store.Claim(actionKey)` (processor.go:100); `Claim` returns true iff `executed[key.String()]` is absent, then inserts a `StatusClaimed` record (action_store.go:108-126). `actionKey = {ID: Metadata.ID, Version: Metadata.Version}` (processor.go:90-93). If Claim returns true the code proceeds to `p.registry.Execute` (processor.go:153) — the actual mutating call.

**Actions are leader-gated and RC-driven.** `ConfigRetriever.actionsCallback` (config_retriever.go:47-85) checks `cr.isLeader()`; non-leaders reply `ApplyStateUnacknowledged{Error:"not the leader"}` (lines 64-70) and never Claim. So only the leader's store is ever populated. Subscription is via `SubscribeIgnoreExpiration(state.ProductK8SActions, ...)` (config_retriever.go:40) — on (re)subscribe after reconnect/restart, RC replays the current config set.

**TTL does not save us.** `ValidateTimestamp`/`isExpired` (action_store.go:78-104) reject only actions older than `ActionTTL = 1m` by wall-clock. A re-delivered action still within its 1-minute window passes validation and, on a fresh store, is claimed and executed again. Backend RC configs typically persist far longer than a failover takes.

## Failure scenario (concrete)
1. Leader A receives K8S_ACTIONS config `{id: act-1, version: 7}` for `restart_deployment payments`. Claim succeeds, deployment restarted, record stored in A's in-memory map.
2. Antithesis partitions A ↔ kube-apiserver for > `leader_lease_duration` (default 60s). A loses the lease; follower B wins.
3. RC (still fresh, <1m or re-pushed) delivers `{id: act-1, version: 7}` to B. B's ActionStore is empty. `Claim` returns true. `restart_deployment payments` executes **again**.

## Instrumentation (MISSING — net-new)
- `assert.Always(executedOnce, "kubeaction executed at most once per (id,version)", details{key})` — but single-process Always cannot see cross-process duplicates. Correct approach: emit the ActionKey to an external witness (Antithesis workload sidecar or the EVP/fakeintake result stream via `reporter.ReportResult`) and assert the count of distinct execution attempts per ActionKey ≤ 1 across the run.
- `assert.Reachable("kubeaction claimed by a replica that is not the original executor", ...)` at processor.go:100 to confirm the harness exercised failover with a pending action.

## Why existing tests miss it
Unit tests use a single in-process store and never simulate failover or RC replay against a second replica (existing-assertions.md: zero SDK instrumentation; §9: leader transitions deferred to E2E, and E2E `testDCALeaderElection` only ever passes `restartLeader=false`).


### from `kubeactions-at-most-once-execution-not-durable`

## Mechanism (verified in source)

**Dedup store is in-memory and non-durable.**
- `ActionStore.executed map[string]ActionRecord` guarded by a `sync.RWMutex`, created fresh by `NewActionStore` (action_store.go:56-75). No k8s/disk backing.
- `Setup` calls `NewActionStore(ctx)` on every subsystem start (setup.go:42); a new leader pod = a new process = an empty store.
- SUT analysis state inventory (§6): kubeactions dedup "in-memory map, wiped on restart — No" persistence.

**Claim/execute is not atomic.**
- `Process` claims then, on success, runs `processAction` which calls `registry.Execute` at processor.go:153 and only then `MarkExecuted` at processor.go:157.
- `Claim` (action_store.go:108-126) inserts `StatusClaimed` immediately; there is no persisted transition to "executed" until after the mutating call returns.
- A crash between :100 (claim) and :157 (mark) leaves NO record at all after restart (RAM wiped) → re-claim → re-execute.

**At-least-once redelivery drives replay.**
- `actionsCallback` re-processes whatever `update map[string]state.RawConfig` RC hands it (config_retriever.go:47-84); it ACKs immediately (:74) and spawns `go processor.Process` per config. A new subscriber (new leader) receives the current config set again.
- TTL gate: `ValidateTimestamp` only rejects actions older than `ActionTTL = 1*time.Minute` (action_store.go:24, :98-101). Any redelivery inside 1 minute passes and executes.

**Non-idempotent executors.**
- `restart_deployment` / `rollback_deployment` registered at setup.go:74-76; a restart patches a rollout-triggering annotation, so a repeat is a distinct second rollout.

## Failure scenario
1. Backend issues action `A` (id=X, v=1, ts=now) → leader pod P1 claims, executes restart_deployment, deployment rolls.
2. Within 60s, P1 loses the lease (apiserver partition ≥ lease duration) or is killed; P2 becomes leader with an empty ActionStore.
3. RC delivers config for `A` to P2; ts still < 60s old → ValidateTimestamp passes → P2 claims (fresh map) → executes restart_deployment AGAIN → second rollout.

Expected assertion `executionCount[X:v1] <= 1` FAILS (observed 2).

## Existing coverage gap
`action_store_test.go` exercises Claim/MarkExecuted within one process only; there is no test that crosses a process boundary or leadership change, so durable at-most-once is unverified (matches SUT analysis §9: leader transitions deferred to E2E, and E2E leader-restart path is dead code).
