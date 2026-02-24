# RCA Evaluation Score Matrix

Date: 2026-02-23
Model: gpt-5.2-2025-12-11
Eval pipeline: observer-demo-v2 (Go) -> analyze_with_llm.py (diagnosis) -> evaluate_diagnosis.py (grading 0-100)

## Architecture: RCA + Compaction Flow to LLM

```
                          OBSERVER PIPELINE (Go)
 ============================================================================

  Parquet Files          Layer 1: Detection        Layer 2: Correlation
 +--------------+       +------------------+       +---------------------+
 | cgroup v2    |       |                  |       |                     |
 | CPU, mem,    +------>+  CUSUM Detector  +------>+ TimeCluster         |
 | I/O, net,    |       |  (shift detect)  |       | Correlator          |
 | kubelet ...  |       |                  |       | (1s proximity)      |
 +--------------+       +------------------+       +--------|------------+
                                                            |
                         Raw anomalies per series      Correlations
                         (source, timestamp, sigma)    (pattern, sources[], window)
                                                            |
                                                            v
                                               Layer 3: RCA Engine
                                          +---------------------------+
                                          |  Temporal Onset Ranking   |
                                          |                           |
                                          |  For each anomalous       |
                                          |  series in a correlation, |
                                          |  compute a root-cause     |
                                          |  score (0-1):             |
                                          |                           |
                                          |  POSITIVE FACTORS:        |
                                          |  onset     * 0.35         |
                                          |    How early did it start |
                                          |    vs others? (earliest   |
                                          |    = 1.0, latest = 0.0)   |
                                          |                           |
                                          |  coverage  * 0.25         |
                                          |    What fraction of other |
                                          |    nodes are downstream?  |
                                          |    (reachable via directed |
                                          |    edges from this node)  |
                                          |                           |
                                          |  persist   * 0.15         |
                                          |    How many times did it  |
                                          |    anomaly? (normalized   |
                                          |    vs max in cluster)     |
                                          |                           |
                                          |  severity  * 0.15         |
                                          |    How large was the      |
                                          |    deviation? (peak sigma |
                                          |    vs max in cluster)     |
                                          |                           |
                                          |  NEGATIVE FACTORS:        |
                                          |  incoming  * 0.25         |
                                          |    Does something else    |
                                          |    point TO this node?    |
                                          |    (if so, it's a symptom |
                                          |    not a root cause)      |
                                          |                           |
                                          |  spread    * 0.15         |
                                          |    Is this node's         |
                                          |    namespace under-       |
                                          |    represented in the     |
                                          |    cluster? (bystander    |
                                          |    penalty — e.g. cleanup |
                                          |    container with 1/4     |
                                          |    nodes vs 3/4 app)      |
                                          |                           |
                                          |  CONFIDENCE FLAGS:        |
                                          |  ambiguous_roots:         |
                                          |    top-2 scores too close |
                                          |    OR onset gap < 10% of  |
                                          |    cluster window          |
                                          |  weak_directionality:     |
                                          |    < 30% of edges have    |
                                          |    inferred direction     |
                                          |  data_limited:            |
                                          |    < 3 nodes or 0 edges   |
                                          |                           |
                                          |  Outputs:                 |
                                          |   - Ranked candidates     |
                                          |   - Evidence paths        |
                                          |   - Confidence (0-1)      |
                                          +---------------------------+
                                                       |
                                          Candidates + Confidence
                                                       |
                                                       v
                                          Layer 4: Correlation Digest
                                          +---------------------------+
                                          |                           |
                                          |  ALWAYS produced:         |
                                          |   - metric_family_counts  |
                                          |   - total_source_count    |
                                          |   - rca_confidence        |
                                          |   - confidence_flags      |
                                          |                           |
                             +------------+--- confidence >= 0.6? ----+-----------+
                             |            |                           |           |
                             v            +---------------------------+           v
                        HIGH CONF                                          LOW CONF
                   +------------------+                              +------------------+
                   | key_sources:     |                              | key_sources:     |
                   |  RCA-ranked top  |                              |  1-2 samples per |
                   |  10 with scores  |                              |  top metric      |
                   |  and Why reasons |                              |  family (no      |
                   |                  |                              |  scores, no      |
                   | onset_chain:     |                              |  causal claim)   |
                   |  temporal order  |                              |                  |
                   |  of earliest     |                              | onset_chain:     |
                   |  anomalies       |                              |  OMITTED         |
                   +------------------+                              +------------------+
                             |                                                |
                             +---------------------+  +-----------------------+
                                                   |  |
                                                   v  v

 ============================================================================
                          COMPACTION RESULT (JSON)
 ============================================================================

  Before (raw):                          After (digest):
  correlations[0].sources: 2477 IDs      correlations[0].sources: 10 IDs
  Total: 1.2 MB                          correlations[0].digest:
                                           metric_family_counts: {37 families}
                                           key_sources: [10 entries]
                                           onset_chain: [0-8 entries]
                                           rca_confidence: 0.53
                                           confidence_flags: [ambiguous_roots]
                                         Total: 44 KB  (97% reduction)

 ============================================================================
                          LLM PROMPT (Python)
 ============================================================================

  analyze_with_llm.py: build_digest_hint(parsed)

  +-----------------------------------------------------------------------+
  | HIGH CONFIDENCE (>= 0.6):                                            |
  |                                                                       |
  |   **Correlation time_cluster_1** (888 series, conf: 0.75)            |
  |     Metric families: cpu.user: 45, mem.usage: 38, io.bytes: 31 ...   |
  |     Onset chain: cpu.user (T=100) -> mem.usage (T=102) -> ...        |
  |     Top 10 ranked root-cause candidates:                             |
  |       - cpu.user (score=0.870) -- earliest onset; high severity      |
  |       - mem.usage (score=0.720) -- high severity                     |
  |     RCA time_cluster_1: top_metric=cpu.user (score=0.87)             |
  |       evidence path: cpu.user -> mem.usage -> io.bytes               |
  |                                                                       |
  |   Instruction: "Trust the ranked root-cause candidates and onset     |
  |   chain."                                                             |
  +-----------------------------------------------------------------------+

  +-----------------------------------------------------------------------+
  | LOW CONFIDENCE (< 0.6):                                              |
  |                                                                       |
  |   **Correlation time_cluster_1** (888 series, conf: 0.53,            |
  |     flags: ambiguous_roots)                                           |
  |     Metric families: cpu.user: 45, mem.usage: 38, io.bytes: 31 ...   |
  |     NOTE: RCA confidence is low (0.53). The causal ordering is       |
  |     uncertain. Focus on metric family breadth and overall anomaly    |
  |     pattern rather than specific root-cause rankings.                 |
  |     Representative series from most impacted families:               |
  |       - cpu.user (representative of 45 cpu.user series)              |
  |       - mem.usage (representative of 38 mem.usage series)            |
  |                                                                       |
  |   Instruction: "Do NOT rely on specific series rankings for          |
  |   causation -- focus on overall metric family breadth."              |
  +-----------------------------------------------------------------------+

                                    |
                                    v
                          +-------------------+
                          |  GPT-5.2 (LLM)   |
                          |                   |
                          |  Diagnosis:       |
                          |  1. Correlations  |
                          |  2. Problem?      |
                          |  3. What?         |
                          |  4. Confidence    |
                          |  5. Alternatives  |
                          |  6. Evidence      |
                          +-------------------+
                                    |
                                    v
                          +-------------------+
                          |  evaluate_        |
                          |  diagnosis.py     |
                          |                   |
                          |  Score: 0-100     |
                          +-------------------+
```

## Score Matrix (5-run averages)

| Scenario | Ground Truth | Baseline (avg) | v1: Suppress | v2: Digest | v3: Conf-Gate (avg) | Delta (v3-base) | Baseline JSON | RCA JSON |
|---|---|---|---|---|---|---|---|---|
| oom-kill | OOM kill (exit 137, 64Mi limit) | **79** | 78 | 75 | **78** | -1 | 20 KB | 22 KB |
| cpu-starvation | CFS throttling from low CPU limits | **74** | 78 | 65 | **71** | -3 | 20 KB | 22 KB |
| traffic-spike | 18x RPS spike overwhelming system | **15** | 15 | 5 | **36** | +21 | 55 KB | 35 KB |
| s3-outage | etcd quorum loss -> S3 cascade | **8** | 5 | 0 | **4** | -4 | 103 KB† | 44 KB |
| memory-leak | Unbounded heap growth -> OOM kill | **85** | — | — | **79** | -6 | 22 KB | 31 KB |

†s3-outage baseline is 1,266 KB uncompressed — too large for GPT-5.2's context window (422K tokens vs 272K limit). The pipeline has no truncation logic; the API call simply fails. A manually truncated file (103 KB) was created for evaluation. The RCA digest compresses it to 44 KB automatically — no manual intervention needed.

v1/v2 are single-run scores (code not preserved for reruns). Baseline and v3 are 5-run averages. memory-leak v1/v2 cells are blank because the intermediate code was not preserved.

### Per-run breakdown (v3: Confidence-Gated Digest)

| Scenario | Baseline R1 | R2 | R3 | R4 | R5 | Avg | RCA R1 | R2 | R3 | R4 | R5 | Avg |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| oom-kill | 78 | 78 | 78 | 82 | 78 | **79** | 78 | 78 | 78 | 78 | 78 | **78** |
| cpu-starvation | 78 | 75 | 78 | 65 | 75 | **74** | 75 | 65 | 65 | 75 | 75 | **71** |
| traffic-spike | 15 | 15 | 15 | 15 | 15 | **15** | 15 | 75 | 8 | 65 | 15 | **36** |
| s3-outage | 5 | 12 | 8 | 8 | 8 | **8** | 5 | 2 | 5 | 2 | 5 | **4** |
| memory-leak | 92 | 80 | 78 | 92 | 82 | **85** | 82 | 82 | 75 | 78 | 78 | **79** |

Notable: traffic-spike RCA has extreme variance (8-75) vs stable baseline (15 across all 5 runs). The RCA digest occasionally triggers the LLM to mention traffic/load as a possibility, earning partial credit in some runs.

## Eval Runs

### v1: RCA + MinConfidence Suppression (`matrix_20260223_112005/`)

**Implementation:** RCA engine improvements (severity factor, spread penalty, onset gap significance) with MinConfidence=0.5 suppression. Low-confidence RCA is filtered from output entirely.

**Result:** Suppression means RCA variant == baseline for most scenarios (suppressed RCA produces identical output). cpu-starvation +3 is the only lift, from a scenario that barely cleared the confidence threshold.

### v2: RCA + Digest (`matrix_digest_20260223/`)

**Implementation:** Replaced suppression with a correlation digest that compresses raw sources using RCA structural analysis. Digest includes RCA-ranked key sources, temporal onset chain, and metric family counts. All scenarios get digest regardless of confidence.

**Result:** Compression worked (s3-outage: 1.2MB -> 37KB) but scores regressed across the board. The digest prominently surfaced misleading RCA candidates:
- traffic-spike: Digest pointed at `cleanup` container (1s earlier onset by coincidence), score 15 -> 5
- s3-outage: Digest pointed at `observer-agent` container, score 15 -> 0
- cpu-starvation: Narrowed LLM focus away from CPU starvation signal, score 75 -> 65

**Root cause of regression:** When RCA confidence is low (~0.5), asserting specific series as root candidates misleads the LLM more than sending raw data.

### v3: RCA + Confidence-Gated Digest (`matrix_confgate_20260223/`)

**Implementation:** Gate digest content on RCA confidence threshold (0.6):
- **High confidence (>= 0.6):** RCA-ranked key sources + temporal onset chain (causal framing)
- **Low confidence (< 0.6):** Representative samples from top metric families, no onset chain, no causal assertions. LLM instructed to reason from metric breadth.

**Result:** All v2 regressions eliminated. Compression preserved (s3-outage: 44KB). All 4 current scenarios fall in low-confidence mode.

## LLM Diagnosis Summaries

### v3 (Confidence-Gated Digest) — RCA variant

| Scenario | Score | Identified? | LLM Conclusion |
|---|---|---|---|
| oom-kill | 78 | partial | Described multi-resource surge consistent with memory growth/working-set blow-up that "may precede OOM behavior," but did not clearly conclude an OOM kill occurred. |
| cpu-starvation | 75 | partial | Attributed to multi-resource runaway/crash-loop burst (PID/memory/I/O surge) with CPU contention/throttling as a symptom, not specifically CPU starvation from insufficient CPU limits. |
| traffic-spike | 15 | no | Concluded there was a short node-wide shared resource contention event (CPU/IO pressure and memory activity) possibly due to a background job or rollout, not a request/traffic spike. |
| s3-outage | 5 | no | Reported a broad, time-aligned node/container runtime + disk/filesystem anomaly cluster. Root cause unclear. Suggested disk events, kubelet/runtime churn, workload spikes, or metrics artifacts. |

### v2 (Digest v1, no confidence gate) — RCA variant

| Scenario | Score | Identified? | LLM Conclusion |
|---|---|---|---|
| oom-kill | 75 | partial | Described container-wide resource surge (CPU/memory/I/O pressure) and suggested it *could* lead to instability/oom conditions. |
| cpu-starvation | 65 | partial | Attributed to runaway process/fork and memory/dirty-page growth driving I/O, CPU pressure/throttling only secondary. |
| traffic-spike | 5 | no | Concluded cluster-wide resource contention likely initiated by a "cleanup" container. **(Misled by digest)** |
| s3-outage | 0 | no | Attributed to "observer-agent" container causing heavy disk writes and CPU/memory pressure. **(Misled by digest)** |

## Implementation Details

### RCA Engine Changes

**Files:** `rca_config.go`, `rca_engine_temporal.go`, `rca_confidence.go`, `rca_service.go`, `rca_types.go`

#### Severity Factor (`rca_engine_temporal.go`)
Ranks series with larger sigma deviations higher. `SeverityFactor = node.PeakScore / maxPeakScore` (normalized 0-1). Weight: 0.15.

#### Container-Spread Penalty (`rca_engine_temporal.go`)
Penalizes candidates in underrepresented namespaces. Uses `parseSeriesKey()` to extract namespace, computes share of graph nodes per namespace. If a candidate's namespace is underrepresented (share < expectedShare), applies penalty proportional to the gap. Weight: 0.15. Only active when cluster spans 2+ namespaces.

#### Onset Gap Significance (`rca_confidence.go`)
Flags `ambiguousRoots` when the onset time gap between top-2 candidates is < 10% of the cluster window span, even if their scores differ. Catches cases like traffic-spike where `cleanup` was 1s ahead in an 8s window.

#### Scoring Formula
```
score = onset*0.35 + coverage*0.25 + persistence*0.15 + severity*0.15
        - incomingPenalty*0.25 - spreadPenalty*0.15
```

### Correlation Digest

**File:** `demo_main_v2.go`

#### CorrelationDigest struct
- `KeySources []DigestSource` — top series (RCA-ranked or family samples)
- `MetricFamilyCounts map[string]int` — all sources grouped by metric name
- `OnsetChain []DigestOnset` — temporal ordering (high-confidence only)
- `TotalSourceCount int` — original source count before compression
- `RCAConfidence float64` — lets LLM calibrate trust
- `ConfidenceFlags []string` — specific uncertainty reasons

#### Confidence Gate (threshold: 0.6)
- **High confidence:** KeySources from RCA ranking (with scores and Why reasons), OnsetChain sorted by onset time
- **Low confidence:** KeySources are representative samples from top metric families (1-2 per family, sorted by family count descending), no OnsetChain, score=0, Why explains representativeness

#### LLM Prompt Framing (`analyze_with_llm.py`)
- High confidence: "Top N ranked root-cause candidates:" with scores and onset chain
- Low confidence: "NOTE: RCA confidence is low. Focus on metric family breadth and overall anomaly pattern." Shows "Representative series from most impacted families:" without scores

### Config Keys (`pkg/config/setup/config.go`)
- `observer.rca.min_confidence` (0.5) — retained but unused after suppression removal
- `observer.rca.weights.severity` (0.15)
- `observer.rca.weights.spread_penalty` (0.15)

### Tests
- `TestSeverityFactorBoostsHighDeviationNodes` — high PeakScore outranks low
- `TestSpreadPenaltyPenalizesUnderrepresentedNamespace` — cleanup bystander penalized
- `TestSpreadPenaltyNoEffectSingleNamespace` — no penalty in single-namespace cluster
- `TestOnsetGapSignificanceFlagsAmbiguity` — 5% gap flags ambiguous
- `TestOnsetGapLargeDoesNotFlagAmbiguity` — 40% gap does not flag
- `TestDigestHighConfidenceIncludesOnsetChainAndRankedSources`
- `TestDigestLowConfidenceOmitsOnsetChainUsesFamilySamples`
- `TestRCAServiceAlwaysEmitsResults` — no suppression

## Scenarios

| Scenario | Parquet Directory | Description |
|---|---|---|
| oom-kill | `scenarios/oom-kill` | Python allocates 10MB chunks until 64Mi limit triggers OOM kill (exit 137) |
| cpu-starvation | `scenarios/crash_loop` | Backend CPU limits too low, CFS throttling active, 22% request timeouts |
| traffic-spike | `scenarios/todo-app-redis-traffic-spike` | 18x RPS spike, 48% success, CPU saturation across services |
| s3-outage | `scenarios/002_AWS_S3_Service_Disruption-Recording_02-18-2026` | Capacity removal -> etcd quorum loss -> S3 cascade failure |

## Observations

1. **Grading variance is high and asymmetric.** Baseline scores are stable (traffic-spike: 15 across all 5 runs) but RCA scores swing wildly (traffic-spike RCA: 8, 15, 15, 65, 75). The digest changes LLM output enough to occasionally trigger different grader interpretations.

2. **RCA is slightly negative on 4 of 5 scenarios.** oom-kill: -1, cpu-starvation: -3, s3-outage: -4, memory-leak: -6. These are small but consistent. The structural summary in low-confidence mode may slightly constrain the LLM's reasoning compared to raw data.

3. **traffic-spike is the outlier.** +21 average, but driven by high variance (the RCA digest occasionally prompts the LLM to mention traffic/load as a possibility). The baseline never gets this — it's stuck at 15 every run. Whether this counts as "lift" or "noise" is ambiguous.

4. **Compression is the primary value.** s3-outage shrank from 1.2MB (exceeded token limit) to 44KB. traffic-spike from 55KB to 35KB. This enables analysis of large incidents that previously failed.

5. **All scenarios have RCA confidence < 0.6.** The high-confidence causal path (ranked sources + onset chain) remains untested on real data.

6. **The hard scenarios remain hard.** traffic-spike (avg 15 baseline) and s3-outage (avg 8 baseline) score low because the ground truths involve phenomena not directly visible in container metrics.
