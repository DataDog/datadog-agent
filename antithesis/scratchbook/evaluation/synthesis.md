---
sut_path: /Users/jon.rosario/go/src/github.com/DataDog/datadog-agent
commit: f2da1471bb748fb5108f89f36f7b83cab305ca79
updated: 2026-07-21
external_references: []
---

# Property Catalog Evaluation — Synthesis

Four evaluation lenses (Antithesis-fit, coverage-balance, implementability, wildcard) stress-tested
the 27-property catalog as a portfolio. Evidence files: `evaluation/{antithesis-fit,coverage-balance,implementability,wildcard}.md`.
Findings below are categorized **Refinement** (apply directly), **Gap** (fill via targeted discovery),
or **Bias** (needs human judgment).

## User steer (recorded)

The user reviewed the priority ranking and confirmed **the P0 properties are the right ones to assert**.
This resolves the largest potential bias (B1 below): the catalog's concentration on the
cluster-check dispatch / leadership control loop is *intentional and correct*, matching where the
real risk and the densest bug history live. The coverage lens's "skew" finding is therefore treated
as a signal to **add** the missing subsystems (gaps G1–G8), not to reduce dispatch/leadership depth.

## Refinements (applied directly to the catalog)

- **R1 — Witness every race-property (catalog-wide).** [antithesis-fit, wildcard] 21/27 assertions are
  Always/AlwaysOrUnreachable; AOU passes green when the guarded path never runs, so a green board is
  indistinguishable from "the hazardous window never opened." **Action:** pair every race-dependent
  safety property with a `Reachable`/`Sometimes` witness asserting the adversarial precondition
  (interleaving/leaderless window/forwarded-loop request/expiry-during-flap) was actually scheduled.
  Added as a standing requirement in the catalog and to each affected property's instrumentation note.

- **R2 — Tag every property by intent (catalog-wide).** [wildcard] ~7 properties assert something the
  code is already read to violate ("expected to FAIL — that is the point"). Mixing true invariants,
  aspirational "should-improve", and known-defect reproducers in one board causes alert fatigue and
  defeats regression detection. **Action:** add an **Intent** field to every property —
  `invariant` / `should-improve` / `known-defect-reproducer`. Known-defect reproducers ship as a
  minimized reproducing trace and flip to a live invariant once fixed.

- **R3 — Reframe `store-lock-bounded-under-slow-clc`.** [antithesis-fit, implementability, wildcard — 3 lenses]
  A wall-clock duration bound is Antithesis-hostile (Antithesis controls the scheduler) and the
  threshold is uncalibrated. **Action:** restate as the structural safety invariant **"the clusterStore
  write lock is never held across the CLC-runner HTTP call"** (a boolean assertable at the network call
  site), dropping the duration bound. Instrumentation note updated (timed-mutex not needed; assert
  not-in-critical-section at the call site).

- **R4 — Invert `graceful-shutdown-releases-lease-bounded`.** [wildcard] The documented bug is that
  ReleaseOnCancel can *hang* under partition; a `Sometimes(released within bound)` only witnesses the
  happy path and can never fail on a hang. **Action:** restate as an `Always`/bounded-time liveness
  assertion that shutdown completes (or the lease is released) within a bound under partition, so the
  hang is what fails. Bump priority P2→P1 (genuine partial-failure timing content).

- **R5 — Reframe `liveness-probe-no-restart-loop`.** [antithesis-fit, wildcard] Redundant with the two
  lock-hazard properties and its impact narrative assumes a kubelet the topology lacks. **Action:**
  restate around the directly-observable internal symptom (health endpoint goes red / probe channel not
  drained while a recoverable delay is in progress); drop the restart→churn-cascade justification unless
  a probe/restart shim is added to the topology (noted as a topology option). Keep as `Liveness` but
  precise-predicate.

- **R6 — Fold `lastconfigchange-monotonic-epoch` into `dispatch-store-bijection`.** [antithesis-fit,
  coverage] Near-inert standalone (needs disabled clock skew) and its real concern (missing
  leader-generation epoch) is already an open question on bijection. **Action:** merge the epoch concern
  into bijection's evidence + open questions; remove the standalone property. (27 → 26 before gap-fill.)

- **R7 — Make backoff cap reachable for `extmetrics-backoff-cap-stays-serving`.** [antithesis-fit,
  implementability] The 1800s cap is a hardcoded package `var`, unreachable in a normal timeline.
  **Action:** override the backoff-policy var in the instrumented test build to shorten the cap; note
  the property is inert on short runs otherwise.

- **R8 — Gate fault-dependent properties explicitly (catalog-wide).** [all four lenses] ~5 properties
  lean on disabled-by-default faults. **Action:** add a **"Requires fault"** marker; where a
  workload-driven substitute exists, make it primary:
  - `forwarder-target-is-live-endpoint`: the workload owns the DCA EndpointSlice → mutate it to a
    stale/dead IP directly; downgrade node-termination to optional.
  - `kubeactions-at-most-once`: lead with the split-brain (partition-only) duplication path; treat
    crash-replay (node termination) as an enhancement.
  - `node-expiry-monotonic-clock`: strictly gate behind clock-fault availability (cannot be
    workload-induced — only the leader stamps heartbeats on receipt); mark inert otherwise.
  - Investigate whether a plain container stop/restart in the compose harness substitutes for
    Antithesis node-termination (would revive the crash-replay paths).

- **R9 — Commit the harness to a short lease (topology + catalog).** [implementability] `leader_lease_duration`
  and `warmup_duration` are config-driven with no floor; a short lease makes flap-dependent preconditions
  (warmup interruption, leadershipchan wedge) deterministic **without** clock-skew/node-termination
  faults. **Action:** stop treating "can client-go flap fast enough?" as an open question — commit
  `deployment-topology.md` to a short `leader_lease_duration` + matching `warmup_duration`.

- **R10 — Reclassify `getconfigs-distinguishes-unknown-node`.** [implementability, wildcard] Today the
  unknown-node branch returns HTTP 500; as an `AlwaysOrUnreachable` invariant it fails against current
  code by design. **Action:** tag intent `known-defect-reproducer` / `should-improve`; restate as
  detecting the ambiguity (500 used for both), and gate its value on the workload modeling node-agent
  backoff-on-error (a workload requirement, not a scope footnote).

- **R11 — Split `reset-restores-store-and-gauges` observation.** [implementability] The gauge half is
  Prometheus telemetry on the unauth'd `metrics_port` — the workload can scrape `/metrics` and assert
  ground-truth across a flap with no instrumentation. **Action:** note gauge-half is workload-observable
  (cheap), reserve SUT instrumentation for the in-memory-map half.

- **R12 — Deterministic/low-Antithesis-value properties.** [antithesis-fit] `isexternalpath-classifier-consistency`,
  `admission-webhook-no-silent-nil-cert` (branch-deterministic), `autoscaling-fatal-startup-crashloop`,
  `empty-token-never-authenticates` (core is a deterministic compare) add little beyond a unit test.
  **Action:** keep but mark **P2 / low Antithesis-fit**, tag intent, and note the unit-test overlap. Keep
  `empty-token`'s startup-ordering-race angle and `isexternalpath` only if an independent expected-auth-class
  oracle is supplied (else it is tautological — flagged as blocker in its open questions).

## Gaps (being filled via targeted discovery — workflow `cluster-agent-gap-fill`)

- **G1 — gRPC stream lifecycle.** [coverage] The tagger + kube-metadata streaming data plane (a
  node-agent-facing surface on the main port, known reconnect/unsubscribe race #48026/#50670) has ZERO
  properties. New property: no leaked/permanently-dropped subscription across stream drop+reconnect and
  leadership flip.
- **G2 — DatadogMetric CRD store divergence.** [coverage] The modern external-metrics path is uncovered
  (existing property covers only the legacy ConfigMap store). New property: leader CRD status and
  follower/in-memory views do not diverge across leadership churn.
- **G3 — No duplicate check execution (fencing) — HIGHEST.** [wildcard] The SUT's #1 timing hazard
  (§8) has no catching property; `dispatch-store-bijection` is correct-by-design while two agents run the
  same check. New **workload-witnessed** property: a config kept running on partitioned node N is never
  simultaneously assigned to another live node without N first being told to drop it. No node termination
  needed (partition only).
- **G4 — ConfigMap concurrent-create race.** [coverage, wildcard] Token + cluster-ID ConfigMaps use
  read-then-create with no conflict guard; two replicas booting simultaneously (the exact topology) can
  diverge. New property: replicas converge on a single token/cluster ID. Needs no special faults —
  Antithesis's core competency.
- **G5 — Rebalance convergence/termination.** [coverage, wildcard] Freshly-landed churny algorithm
  (#52884). New pair: each cycle terminates (liveness) + no config oscillation under stale busyness (safety).
- **G6 — Informer cache freeze root-cause.** [coverage, wildcard] `informer_client_timeout=0` freezes
  the cache silently under a watch-drop partition; underlies several symptom properties. New root-cause
  property: DCA does not silently serve authoritative stale data from a frozen informer.
- **G7 — Admission webhook availability under fault.** [coverage] `failurePolicy=Fail` fail-closed blast
  radius is only an open question today. New property (DCA-side): the DCA that should serve the webhook
  actually responds across leadership churn / apiserver partition.
- **G8 — Forwarder path/header fidelity.** [coverage] RequestURI path-restoration (§10 claimed-untested)
  has no property. New property: forwarded path + Authorization arrive intact at the leader.

## Biases (escalated to the user — RESOLVED)

- **B1 — Portfolio concentration on dispatch/leadership. → RESOLVED** by user steer (P0s confirmed
  correct). Resolution: "add gaps, don't reduce depth."
- **B2 — SUT-instrumentation cost / strategy. → RESOLVED: build the custom instrumented DCA image up
  front.** [implementability] Most P0/P1 invariants assert on unexported, build-tag-gated
  (`clusterchecks`, `kubeapiserver`, `!windows`) in-process state. **User decision:** add the Antithesis
  Go SDK to the root `github.com/DataDog/datadog-agent` go.mod and build a custom Linux-only instrumented
  DCA image now, so the P0 in-process invariants (`dispatch-implies-lease-holder`,
  `dispatch-store-bijection`, the lock hazards) are testable in the first run. Consequence: the catalog
  no longer hedges for a "workload-only phase"; in-process instrumentation is a committed prerequisite.
  Build must happen on Linux (not compile-checkable on the macOS dev host); watch for Bazel/Gazelle
  flavor complications when adding the SDK to the root module.
  **Update (antithesis-setup, 2026-08-04):** the SDK-in-root-go.mod + hand-written `assert.*` calls part
  of this decision is done and verified working end-to-end. The *mechanism* changed: `antithesis-go-instrumentor`
  (coverage instrumentation + static assertion cataloging) could not run cleanly against the whole
  ~11,400-file module and was dropped — see `deployment-topology.md` "Instrumentation decision" for the
  investigation and the `blt/antithesis-harness` precedent this followed. "Instrumented image" now means
  "SDK-linked, plain `go build`," not coverage-instrumented; every `assert.*` call still fires and reports
  at runtime regardless.
- **B3 — Cross-replica observation. → RESOLVED: SUT-side instrumentation exports each replica's
  "am-dispatching" fact.** [implementability] `GET /api/v1/clusterchecks` is auth-classified NON-external
  (needs the local IPC token, not the workload's DCA token). **User decision:** rather than mint a shared
  IPC token, add an in-process assertion/counter on each replica that exports its dispatch/leader state
  for global aggregation. Consistent with B2 (instrumented image is being built anyway) and more faithful
  than a token workaround.

## Scope decision (RESOLVED)

- **Orchestrator** (k8s resource collection → Datadog) is **OUT OF SCOPE** for this harness (user
  decision). It is a largely one-directional collection pipeline with less split-brain surface than the
  dispatch/leadership/HPA/admission/gRPC areas. Documented as an explicit exclusion in `sut-analysis.md`.

## Passes (evaluation confirmed these look correct)

- Crown-jewel concurrency/partition properties are correctly typed and genuinely need Antithesis's state
  space: `dispatch-implies-lease-holder`, `dispatch-store-bijection`, `leadershipchan-no-wedge-under-lock`,
  `leader-eventually-dispatches-after-warmup`.
- `Sometimes` used correctly for liveness/progress conditions; `AlwaysOrUnreachable` used correctly for
  optional/feature-gated paths (the caveat is R1's missing witnesses, not mis-typing).
- `dispatch-store-bijection` and `reset-restores-store-and-gauges` reach their hazard via node-agent
  partition (enabled by default) — not dependent on disabled faults.

## Re-evaluation plan

The gap-fill adds ~8 properties (a new category: gRPC/metadata data plane, plus CRD store and
concurrent-create). Per the evaluation reference, a new *category* warrants a light second pass to
confirm integration — done inline during integration (below); refinements R1–R12 are applied regardless.

## Post-integration status (final)

- **Catalog now 37 properties across 11 categories** (was 27 across 9). The 11 gap properties (G1–G8) were
  drafted by a targeted discovery ensemble, validated, and folded in with evidence files; the two new
  categories are **gRPC Streaming Data Plane** and **External Dependency / Informer Freshness**.
- **Refinements R1–R12 applied.** Notably: R1 witnesses are attached as a **Witness** row on every
  race-dependent safety property (and the gap agents authored paired `Reachable`/`Sometimes` witnesses
  directly); R2 **Intent** tags (`invariant` / `should-improve` / `known-defect-reproducer`) on all 37; R3
  store-lock reframed to a structural "no I/O under the lock" invariant; R4 graceful-shutdown inverted to an
  `Always`/bounded assertion; R5 liveness-probe reframed to the observable symptom; R6 `lastconfigchange`
  folded into `dispatch-store-bijection`; R8 fault-gating corrected so only genuinely-inert properties
  (node-expiry) carry the ⚠ marker.
- **Open-Questions investigation pass run** (batched by category) — resolved questions dropped out with an
  Investigation Log entry recorded in each evidence file; remaining questions carry `(partial)` /
  `(needs human input)` tags. This closes the "Investigate Open Questions" step the first draft skipped.
- **Biases + scope all resolved by user** (B1/B2/B3 + orchestrator, above).
- A light second evaluation pass was folded in rather than run as a separate ensemble, since the additions
  are a coherent extension of the existing risk map and the user validated the P0 concentration.

## Post-review human-input round (2026-07-21)

After the OQ investigation, 24 `(needs human input)` questions were grouped and 3 substantive judgment calls
put to the user; the rest were pure harness/deployment config choices deferred to `antithesis-setup` (see
`deployment-topology.md` "Deferred decisions"). Resolutions:

- **`no-duplicate-check-execution-fencing` is a confirmed real defect, not an accepted tradeoff.** The
  finding should be escalated toward adding real fencing (generation/epoch token) to the dispatch protocol.
  Confirms `known-defect-reproducer` intent (no downgrade). Property + evidence file updated.
- **External-metrics provider: DatadogMetric CRD is primary**, not the code-default legacy ConfigMap store.
  `extmetrics-crd-store-converges-after-flip` / `extmetrics-crd-status-no-regression-across-flip` are now the
  primary properties for this slice; `extmetrics-configmap-no-lost-update` is secondary/optional.
- **Remaining config-pinning questions deferred to `antithesis-setup`** (webhook selector/failurePolicy,
  deployment kind, fault-config request, rebalance/warmup tuning, cluster-ID ConfigMap pre-creation).
