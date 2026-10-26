# GenSim global 1 Hz corpus update and rehabilitation record

> Local draft for review. This document does not change the frozen primary corpus, Scenario Store membership, treatment assignments, or the DDEval run occurring in a separate session. It is not yet synchronized to Confluence.

Status date: **2026-09-21**

## Current state

- The original experiment captured and archived **15 matched scenarios / 30 arms**.
- Admission retained **24 primary arms / 12 complete pairs**.
- One independently usable arm, episode 2483 B, remains supplementary and is not in the primary dataset.
- Five arms remain excluded: 2467 A/B, 2483 A, and 2601 A/B.
- The primary Scenario Store dataset is complete and independently verified:
  - Name: `global-1hz-alert-eval-primary-v2-20260917`
  - UUID: `30853552-3a2a-49bf-acf9-db94c74e40c4`
  - Membership: 24 scenarios, all `user_added`, none excluded
- DDEval is now being run separately. This update does not inspect, influence, or report those outcomes.

The current primary cohort is therefore stable. Rehabilitation and expansion are additive upstream workstreams; neither changes the inputs of the DDEval run already in progress.

## General summary of GenSim usage

### 1. Source qualification and outcome-blind freezing

The work began by screening archived CausalGen episodes for runnable ground truth, provenance, Agent-owned telemetry, and a plausible relationship between source emission cadence and the Agent metric-resolution toggle. Fifteen scenarios were selected without inspecting treatment outcomes. Their source bundles were pinned by episode, version, archive hash, canonical scenario path, and scenario hash.

Each selected source was transformed into a matched pair. The shared adaptation installed one bounded Agent deployment, pinned the Agent image, excluded only NTP, and changed only the approved metric-resolution setting between arms. Treatment assignment remained in a private evaluator-only map and was not written to GenSim, GS Admin, Scenario Store, or Bits-visible inputs.

Primary implementation references:

- GenSim derivative ledger at commit `bb927e366703e8ea740358941e47d3c56ddb59b8`:
  `src/controller/campaigns/global-1hz-derivative-change-ledger-v3.json`
- Frozen corpus source record:
  `/Users/ella.taira/episode-bundles/global-1hz-corpus-freeze-sources/source-manifest.json`
- Frozen derivatives:
  `/Users/ella.taira/episode-bundles/global-1hz-execution-derivatives-v3`

### 2. Serial execution and cleanup

The paired derivatives were executed through the direct `episode-ctl` campaign path in a dedicated Datadog Workspace. Scientific execution was serial (`max_parallel=1`) with automatic retry disabled. Every attempt retained its runtime identity, result, cleanup outcome, and failure evidence. Failed or ambiguous operations were reconciled by durable identity rather than replayed.

The execution campaign produced successful captures as well as source, infrastructure, cleanup, and observability failures. Those failures were preserved instead of being rewritten as successful evidence. This distinction matters: a replay can succeed while archival, telemetry collection, event identity, or a later verifier still fails.

Representative campaign evidence:

- Run matrix:
  `/Users/ella.taira/.pi/agent/worktrees/gensim/campaign-core-90f057e2/src/controller/campaigns/global-1hz-campaign-run-matrix.json`
- Original and recovered execution evidence:
  `/Users/ella.taira/episode-bundles/global-1hz-campaign-evidence`

### 3. Archival, recovery, and direct validation

Archival was treated as a separate write boundary from scenario execution. Successful replay alone was not sufficient. Archive workflows were run serially, read back from immutable S3 prefixes, and validated against metadata references. Recovery runs repaired incomplete archive tracks without modifying the original execution evidence.

The final archived population contained 30 arms, but archival success did not imply DDEval admission. Event identity and diagnostic telemetry were checked afterward as independent gates.

Admission evidence:

- Cohort plan:
  `/Users/ella.taira/episode-bundles/global-1hz-campaign-evidence/ddeval-corpus-prep-continuation-20260910T183856Z/manifests/cohort-plan.md`
- Read-only ingestion preflight:
  `/Users/ella.taira/episode-bundles/global-1hz-campaign-evidence/eval-data-portal-ingest-v2-preflight-20260918T141449Z`

### 4. Scenario Store ingestion

The 24 eligible primary arms were ingested from exact immutable S3 prefixes with `force=false`. Scenario creation and dataset membership were handled as separate operations. Each scenario UUID and each manual dataset link were read back. The final verifier checked the exact UUID set, exact S3 path set, mapping origin, privacy boundary, and absence of supplementary episode 2483 B.

Verified evidence:

- Final continuation summary:
  `/Users/ella.taira/episode-bundles/global-1hz-campaign-evidence/eval-data-portal-primary-continuation-20260918T154956Z/summary.json`
- Independent readback:
  `/Users/ella.taira/episode-bundles/global-1hz-campaign-evidence/eval-data-portal-primary-continuation-20260918T154956Z/final-independent-readback-v2.json`
- Authorization-bound manifest SHA-256:
  `51857bfe468b079f234177a223a2c1744cd0a4bb4b157164bce7611d8be950a9`
- Aggregate request SHA-256:
  `c2f7cb837a7d006f3ac2365b82d2a32ae8cd9f64b8f9af752929ed1423b36b28`

### 5. Additive and managed-production usage

Two additional forms of GenSim usage were investigated separately from the primary experiment:

1. **Additive existing episodes.** Episodes 2473 and 2479 completed paired direct executions. Episode 2487 failed before scenario execution because its Kafka Helm hook could not schedule and the controller imposed a fixed timeout; it was classified as a source defect rather than a treatment outcome.
2. **Managed QBranch canaries.** Production canaries exercised the public `GenerateCausalgenEpisodes` dispatcher, worker-owned execution lifecycle, gs-flow, verifier, result tagging, archival boundary, and duplicate protection.

The managed canaries found operational defects without adding scientific observations:

- Episode 14935 demonstrated the deployed bootstrap path, then was externally canceled while gs-flow remained live. Its execution became stale in GS Admin and fresh claims were correctly rejected.
- Episode 15080 replayed successfully twice. The first managed validation failed on a post-replay Logs HTTP 408 and separately failed to persist result tags with HTTP 403. The second did not reproduce the 408 and did persist tags, but failed the required `alert_fired` and `logs_exist` checks. Neither execution archived.
- Episode 15285, created from the frozen episode-2563 derivative for an operational canary, failed before runtime. Worker build `4e156fc9a53d9c9fe90c46a560dd6b5a` attempted `causalgen.state: "" -> "running"`, which GS Admin rejected as `IllegalStateTransition`. No gs-flow job was created, so this run produced no scenario or alert evidence.

These canaries show why replay success, managed validation success, archival success, and Scenario Store admission must remain separate states.

Evidence:

- Episode 14935 cancellation diagnosis:
  `/Users/ella.taira/episode-bundles/global-1hz-campaign-evidence/prod-managed-canary-14935-cancellation-diagnosis-20260918T180531Z`
- Episode 15080 repeat evidence:
  `/Users/ella.taira/episode-bundles/global-1hz-campaign-evidence/prod-managed-canary-15080-repeat-20260918T203306Z`
- Episode 15285 lifecycle failure:
  `/Users/ella.taira/episode-bundles/global-1hz-campaign-evidence/prod-managed-primary-2563-canary-20260921T141025Z`

## What rehabilitation means

**Rehabilitation means producing new valid execution evidence upstream.** It does not mean relabeling, editing, importing, or reusing an excluded archive. Historical exclusions remain immutable evidence even if a new version later passes admission.

A rehabilitation attempt must use:

1. A new versioned source or derivative with immutable source and transformation hashes.
2. A matched A/B pair that differs only by the approved Agent-resolution treatment.
3. Fresh serial execution with no outcome-based retry.
4. Unique trigger and recovery event identity for each arm.
5. Usable diagnostic telemetry in at least one supported track.
6. Complete cleanup, immutable archival, direct S3 validation, exact Scenario Store projection validation, and independent readback.
7. A new additive admission decision; no mutation of the original excluded evidence.

## Rehabilitation matrix

| Episode | Missing arms | Preserved admission defect | Required rehabilitation evidence | Historical evidence that must remain excluded |
|---:|---|---|---|---|
| 2467 | A and B | Neither archive has an exact alert-transition event identity. Both were classified `technical_event_ineligible_no_transition`. | Execute and archive a fresh matched pair in which trigger and recovery transitions are uniquely attributable to each arm. Validate event identity before Scenario Store ingestion. | `001-pre-rendered-2467-611aea`; `002-pre-rendered-2467-ac5659` |
| 2483 | A, with B supplementary only | Arm A's exact trigger or recovery event was not unique (`technical_event_ineligible_evidence_unavailable`). Arm B is independently usable but has no eligible partner. Operator-provided current status says a later paired replication also failed to establish unique event identity; durable sealing of that replication remains outstanding. | Produce a new matched pair with unique per-arm trigger and recovery identities. Both new arms must independently pass admission; the existing supplementary B arm cannot serve as the partner. | `005-pre-rendered-2483-cc42a6`; supplementary `006-pre-rendered-2483-8b20b0`; preserve the unsealed replication separately |
| 2601 | A and B | Both corrected archives contain zero usable logs, traces, metrics, deployments, or profiles (`telemetry_empty`). Exact alert events alone are insufficient for RCA evaluation. | Execute and archive a fresh matched pair that emits usable diagnostic telemetry while preserving the scenario's objective ground truth and matched treatment boundary. | `001-pre-rendered-2601-0e9cde`; `002-pre-rendered-2601-ac1de0`; malformed and corrected-source attempts |

Evidence boundaries:

- Arm-level admission findings:
  `/Users/ella.taira/episode-bundles/global-1hz-campaign-evidence/ddeval-corpus-prep-continuation-20260910T183856Z/manifests/cohort-plan.md`
- The session checkpoint records replicated-2483 cleanup and sealing as still outstanding; the replication-failure statement above is therefore explicitly operator-provided rather than treated as sealed admission evidence:
  `/Users/ella.taira/Documents/Obsidian Vault/ella.taira/agent-journal/sessions/2026/08/2026-08-21-pi-90f057e2-0dcc-4b17-ba4c-68e077bbb1e8.md`
- Corrected 2601 archive evidence:
  `/Users/ella.taira/episode-bundles/global-1hz-campaign-evidence/archive-2601-corrected-v1-20260910T133205Z/archive-receipts/archive-summary.json`

## Expansion boundary

New incident-grounded candidates are not rehabilitations of these three episodes. They form a separate additive workstream. A source-material candidate enters the active corpus only after it has:

- explicit incident provenance,
- outcome-blind evidence classification,
- a runnable Agent-bearing materialization,
- a frozen matched derivative pair,
- fresh managed or direct execution evidence,
- unique events and usable diagnostic telemetry,
- immutable archive and Scenario Store validation.

The read-only candidate assessment and proposed expanded matrix are maintained separately in:

`docs/dev/gensim-global-1hz-corpus-matrix-v2-draft.md`

## Non-actions in this update

- No Confluence page was changed.
- No existing corpus matrix was changed.
- No Scenario Store scenario or dataset mapping was changed.
- No DDEval, Bits, judge, or scoring call was made.
- No episode was published, dispatched, retried, archived, or ingested.
- No historical evidence was relabeled or removed.
