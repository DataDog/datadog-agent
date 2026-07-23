---
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-29
external_references:
  - path: https://datadoghq.atlassian.net/wiki/spaces/~602449d8f3d296006864db68/pages/6495210537/Property+testing+Logs+Agent+Adaptive+Sampling
    why: Owner's design doc; sampling correctness properties under evaluation.
  - path: https://datadoghq.atlassian.net/wiki/spaces/~712020006700eab4c247639d448c47103cd8b7/pages/6273073381/Logs+to+Disk+-+Payload+Journaling+Design
    why: Backpressure drop points / auditor offset behavior evaluated for coverage.
  - path: https://datadoghq.atlassian.net/wiki/spaces/AL/pages/6782419701/RFC-+Logs+Agent+Backpressure+Status
    why: Backpressure-before-drop claim evaluated against the catalog.
---

# Property Evaluation — Synthesis

Four evaluation lenses (antithesis-fit, coverage-balance, implementability, wildcard)
stress-tested the 36-property catalog as a portfolio. Detailed evidence:
`evaluation/{antithesis-fit,coverage-balance,implementability,wildcard}.md`.
Categorization below (Gap / Bias / Refinement) is the synthesis step's call.

## Headline cross-lens convergence

**The fakeintake fidelity problem (3 lenses: wildcard F1, implementability F9,
coverage).** `test/fakeintake/server/server.go` stores every payload *before*
applying the `ResponseOverride` status code, and `api.Payload` has no field for the
status code returned, and the store does not deduplicate. Consequence: at the
fakeintake query layer, "agent retried" vs "agent dropped" vs "benign restart
replay" are **indistinguishable**. This silently undermines the entire 4xx /
offset / at-least-once / no-loss-and-duplicate family — a large fraction of the
catalog. Treated as a **prerequisite Refinement** (see R-FAKEINTAKE): fakeintake
must (a) record the response code per payload, (b) record/respect ordering of
store-vs-respond, and (c) the workload must use embedded **sequence numbers** (not
body equality) as the dedup/correlation key. This is the single most important
evaluation outcome.

**The monotonic-clock gate (all 4 lenses).** Four properties (Cluster 7 + the two
clock-sampling properties) depend on the Antithesis clock fault moving Go's
*monotonic* clock; Go's `time.Now().Sub`, `time.Since`, `time.Timer`, and
`time.After` all use the monotonic component. If the fault only moves wall-clock
(`CLOCK_REALTIME`), these properties are vacuously true. Escalated as **Bias/Prereq
B-CLOCK** — needs Antithesis-tenant confirmation; if wall-only, reformulate via the
sampler/backoff `now`-injection points as custom faults.

---

## Refinements (applied directly to catalog/evidence)

- **R-ORDERING — `per-source-ordering-preserved`.** Reframe: assert the *real*
  guarantee (per-session/single-tailer ordering) as `Always`; demote the
  cross-rotation and failover breaks to `Sometimes`/reachability (failover is
  config-gated and off by default, so `Sometimes("failover-routing-triggered")`
  would register as unmet under the default topology). [fit F6, impl F5, wildcard F2]
- **R-SECRETS — `secrets-redacted-before-send`.** Drop the SUT-side
  `Unreachable("any bypass path")` (a runtime assertion can't prove the absence of a
  bypass). Replace with: workload-side `Always` (no received body contains the
  sentinel) + a *positive* SUT-side `Always` at the processor output that content was
  transformed when the source has `MaskSequences` configured. [impl F4]
- **R-SAMPLER-EXCLUDE — `high-value-never-sampled`.** Close the backwards open
  question: code confirms `Exclude` ⇒ "never rate-limit" (`shouldSample=false`), so
  it does not conflict with `isImportant` (double-protection, not a drop). [wildcard F8]
- **R-GOROUTINE — `no-goroutine-leak-after-stop`.** The expvar/pprof server binds
  loopback, unreachable cross-container; use a SUT-side `runtime.NumGoroutine()`
  SDK assertion instead of cross-container pprof polling. [impl F3]
- **R-ALIASING — `adaptive-sampler-no-aliasing`.** Note that the Antithesis value is
  input-sequence *diversity*, not thread interleaving (CPU-pause is a weak trigger
  for a sequential-logic bug); recommend a parallel Go fuzz test as the better tool;
  lower its Antithesis priority. [fit F1, impl F14]
- **R-ENCODE — `batch-encode-failure-no-silent-batch-loss`.** Already applied during
  the investigation pass: assertion is `AlwaysOrUnreachable`, not `Reachable`. [fit F2]
- **R-GATES — catalog preamble.** Add a **fault-availability & config-gate matrix**
  making the vacuity exposure machine-checkable: node-termination (Category C +
  3 more), clock (4), `AdaptiveSampler` (5), `pipeline_failover` (1–2),
  `container_collect_all` + container source (3), non-atomic registry (1). Treat A2
  and A7 as hard run-time preconditions, not soft assumptions. [fit F3/F11, coverage GAP-8/IMB-1]
- **R-FAKEINTAKE — A6 + topology.** Record the fakeintake fidelity requirement in
  catalog A6 and `deployment-topology.md` as prerequisite work (response-code per
  payload; sequence-number dedup key). [headline]
- **R-TTL — `at-least-once-no-loss`, `registry-recovers-after-crash` + topology.**
  Pin `logs_config.registry_ttl` (~23h default) well above the max simulated fault
  window so a blocked source's registry entry isn't TTL-evicted (which would make a
  tailer restart from EOF and look like correct behavior while violating
  at-least-once). Add a `Reachable("registry-entry-ttl-evicted")` bug trap. [wildcard F12]
- **R-SENTINELS — reachability sentinels.** Add named `Reachable`/`Sometimes`
  sentinels so safety properties over rare branches don't pass vacuously:
  `closeTimeout-goroutine-fired` (backpressure-no-rotation-loss),
  `noop-sink-nonblocking-drop` (bounded-memory), `tailer-lastReadOffset-advanced-after-backpressure-clear`
  (backpressure-before-drop), `agent-started-with-non-empty-registry` (recovery). [coverage GAP-9/GAP-10, impl F13]

## Gaps (filled via targeted discovery — see new properties)

A targeted discovery pass added these properties (catalog Category J — Evaluation
gap-fill; evidence files written):

- **G1 `processor-render-error-no-silent-loss`** — processor `Render()`/`Encode()`
  errors (`processor.go:198-215`) drop a message with only `log.Error`, no metric,
  no offset advance — distinct from the batch-layer encode property. [coverage GAP-3, wildcard F3]
- **G2 `wildcard-file-ordering-stable`** — `applyReverseLexicographicalOrdering`
  (`file_provider.go:362`) FIXME + skipped test; glob-order assumption at the
  `filesLimit` cap. [coverage GAP-4]
- **G3 `rotation-pipeline-reassignment-no-interleaving`** — the compound
  rotation→pipeline-reassignment→offset interaction that no single existing property
  covers (each covers a pair). [coverage GAP-6]
- **G4 `log-metadata-not-corrupted`** — the catalog is loss-biased; this covers
  `service`/`env`/`ddsource`/status-tag correctness and stale sampler tags — a
  higher-probability production failure than byte corruption. [wildcard F6]
- **G5 `journald-cursor-recovery-no-gap`** — journald cursor recovery (regression
  target `55c63957d9f`); **topology-extension required** (journald source). [coverage GAP-1]
- **G6 `mrf-unreliable-destination-drop-bounded`** — MRF/dual-shipping
  `NonBlockingSend` silent drop on a full unreliable-destination buffer; the
  unreliable-destination drop path (S5) had no property. **Topology-extension
  required** (second intake / MRF enabled). [wildcard F7, coverage]

Container-source compound gap (coverage GAP-7) was folded into
`container-identifier-no-collision` as a refinement note (clock-skew defeats the
timestamp guard during churn) rather than a new property, to avoid duplication.

## Biases (escalated to the user — judgment calls)

These are systematic orientations the evaluators can identify but not resolve.
Presented to the user with evidence:

- **B-CLOCK** — 4 clock properties may be entirely vacuous depending on whether the
  Antithesis clock fault moves Go's monotonic clock. Needs tenant confirmation;
  decides whether to keep, reformulate (custom fault via `now`-injection), or drop.
- **B-TRANSPORT** — the catalog is HTTP-biased (A1). TCP has a *different* offset
  rule (no advance on permanent drop), a `handleServerClose` goroutine leak, and
  defer-cancel accumulation — none covered. Is TCP in scope?
- **B-CONTAINER** — the catalog/topology centers the file tailer; production is
  increasingly containers/k8s. Three container properties are vacuous in the
  file-only topology. Add a Phase-2 container-source topology variant?
- **B-SAMPLING-GA** — adaptive sampling is `experimental_`/non-GA; 5 properties
  (14%) target it. Worth that investment before GA / is it deployed anywhere?

## Bias decisions (user, 2026-05-29)

- **B-CONTAINER → ACCEPTED: add container sources now.** The topology gains a
  container/containerd source container; `container-collect-all-startup-race`,
  `container-addremovesource-ordering`, `journald-cursor-recovery-no-gap`, and
  `mrf-unreliable-destination-drop-bounded` are now in-scope (no longer Phase-2).
- **B-TRANSPORT → ACCEPTED: add TCP coverage.** A1 is relaxed; add TCP-path
  properties — `tcp-permanent-error-no-offset-advance` (TCP does NOT advance the
  offset on permanent drop, vs HTTP), and `tcp-connection-goroutine-no-leak`
  (`handleServerClose` leak `connection_manager.go:125`; defer-cancel accumulation
  `connection_manager.go:102-103`). A TCP topology variant is added. (Evidence files
  to be authored alongside the harness build; code anchors captured here and in
  `evaluation/coverage-balance.md` GAP-2.)
- **B-SAMPLING-GA → ACCEPTED: keep at full priority.** The 5 sampling properties
  stay first-class; the topology enables `AdaptiveSampler`. (The
  `adaptive-sampler-no-aliasing` fuzz-tool note stands as a *tooling* observation,
  independent of priority.)
- **B-CLOCK → ACCEPTED: drop clock properties.** `clock-jump-no-extra-sampling` and
  `clock-jump-no-backoff-underflow` marked DROPPED; the clock *variants* of
  `multiline-not-split-across-pipelines` and `container-identifier-no-collision`
  removed (both survive via rotation/scheduling, not the clock fault). Gate matrix
  updated.

## Passes (consensus the lenses agree are well-formed)

- The auditor-offset + registry-durability clusters (B, C): correct Safety/Liveness/
  Reachability mix; `Unreachable` correctly used for silent-loss paths.
- The backpressure headline pair (`backpressure-no-rotation-loss` +
  `backpressure-before-drop`): cleanly partitions the rotation/no-rotation fault space.
- The shutdown-concurrency hazards (H1–H6 → Cluster 5): each maps to a property.
- Protocol classification pair (`permanent-error-no-retry` + `retryable-no-retry-after`).
- The relationships file's hub analysis (auditor offset; closeTimeout/rotation; clock).

## Second-pass note

The gap-fill added 6 properties (one new category). Per the evaluation guidance this
is "substantial," but the additions are mostly low-coupling extensions of existing
clusters (processor drop ↔ batch-encode; metadata ↔ data-fidelity; journald/MRF are
topology-gated extensions). A lightweight integration check was done (relationships
updated, gates matrix covers them); a full second ensemble pass was judged
unnecessary. Re-run evaluation if the user accepts B-TRANSPORT or B-CONTAINER (which
would add a whole transport/topology dimension).
