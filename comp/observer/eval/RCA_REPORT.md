# Adding Root Cause Analysis to the Observer

## Motivation

The observer pipeline detects anomalies in container metrics and groups co-occurring anomalies into correlations. These correlations are then sent to an LLM for diagnosis.

The problem is how we package correlation results for the LLM. A single incident can produce thousands of correlated anomalous series. Currently, the pipeline sends all correlated series IDs to the LLM. For small incidents this works, but for large ones the payload exceeds token limits entirely. Random sampling to fit within limits risks dropping the root cause series — which may be a single entry among thousands — while retaining noisy symptoms.

Even when the data fits without sampling, the LLM receives a flat, unstructured list. It has to infer which metrics matter, what started first, and what caused what — from a wall of identifiers. This is asking the LLM to do work that the observer already has the data to answer.

The goal of RCA is to solve both problems:

1. **Structure**: Rank which series are likely root causes vs symptoms, infer temporal ordering, and group metrics into families — giving the LLM an organized starting point instead of a flat list.

2. **Compression**: Replace thousands of raw series IDs with a compact digest of key sources, metric family counts, and temporal chains — reducing payload size while preserving diagnostic information.

## Architecture

The observer pipeline is organized in layers. RCA adds Layers 3 and 4 to the existing detection and correlation stack:

```
  Parquet Files           Layer 1: Detection         Layer 2: Correlation
 +--------------+        +------------------+        +---------------------+
 | cgroup v2    |        |                  |        |                     |
 | CPU, mem,    +------->+  CUSUM Detector  +------->+ TimeCluster         |
 | I/O, net,    |        |  (shift detect)  |        | Correlator          |
 | kubelet ...  |        |                  |        | (1s proximity)      |
 +--------------+        +------------------+        +--------|------------+
                                                              |
                          Per-series anomalies           Correlations
                          (source, time, sigma)          (pattern, sources[], window)
                                                              |
                                                              v
                                                Layer 3: RCA Engine
                                           +---------------------------+
                                           |                           |
                                           |  Builds an IncidentGraph  |
                                           |  from each correlation:   |
                                           |                           |
                                           |  - Nodes = anomalous      |
                                           |    series (onset time,    |
                                           |    persistence, peak      |
                                           |    sigma)                 |
                                           |                           |
                                           |  - Edges = temporal       |
                                           |    proximity (A started   |
                                           |    before B within lag    |
                                           |    window => directed     |
                                           |    edge A -> B)           |
                                           |                           |
                                           |  Scores each node as a   |
                                           |  root-cause candidate     |
                                           |  (details below)          |
                                           |                           |
                                           |  Outputs:                 |
                                           |   - Ranked candidates     |
                                           |   - Evidence paths        |
                                           |   - Confidence score      |
                                           +---------------------------+
                                                        |
                                           Candidates + Confidence
                                                        |
                                                        v
                                           Layer 4: Correlation Digest
                                           +---------------------------+
                                           |                           |
                                           |  Compresses correlation   |
                                           |  sources into LLM-ready   |
                                           |  format. Content varies   |
                                           |  by confidence:           |
                                           |                           |
                              +------------+--- confidence >= 0.6? ----+-----------+
                              |            |                           |           |
                              v            +---------------------------+           v
                         HIGH CONF                                          LOW CONF
                    +------------------+                              +------------------+
                    | key_sources:     |                              | key_sources:     |
                    |  RCA-ranked top  |                              |  1-2 samples per |
                    |  10 with scores  |                              |  top metric      |
                    |  and reasons     |                              |  family (breadth |
                    |                  |                              |  not causation)  |
                    | onset_chain:     |                              |                  |
                    |  temporal order  |                              | onset_chain:     |
                    |  of earliest     |                              |  OMITTED         |
                    |  anomalies       |                              |                  |
                    +------------------+                              +------------------+
                              |                                                |
                              +----------------------+  +----------------------+
                                                     |  |
                                                     v  v
                                           +---------------------------+
                                           |  ALWAYS included:         |
                                           |   metric_family_counts    |
                                           |   total_source_count      |
                                           |   rca_confidence          |
                                           |   confidence_flags        |
                                           +---------------------------+
                                                        |
                                                        v

 ============================================================================
                           JSON Output (compacted)
 ============================================================================

   Before digest:                         After digest:
   correlations[0].sources: 2477 IDs      correlations[0].sources: 10 IDs
                                            correlations[0].digest:
                                            metric_family_counts: {37 families}
                                            key_sources: [10 entries]
                                            onset_chain: [0-8 entries]
                                            rca_confidence: 0.53
                                            confidence_flags: [ambiguous_roots]

 ============================================================================
                           LLM Prompt Construction
 ============================================================================

   analyze_with_llm.py reads the JSON and builds a structured preamble:

   HIGH CONFIDENCE (>= 0.6):
   +--------------------------------------------------------------------+
   | Correlation time_cluster_1 (888 series, conf: 0.75)               |
   |   Metric families: cpu.user: 45, mem.usage: 38, io.bytes: 31 ... |
   |   Onset chain: cpu.user (T=100) -> mem.usage (T=102) -> ...       |
   |   Top 10 ranked root-cause candidates:                            |
   |     - cpu.user (score=0.870) -- earliest onset; high severity     |
   |   Instruction: "Trust the ranked candidates and onset chain."     |
   +--------------------------------------------------------------------+

   LOW CONFIDENCE (< 0.6):
   +--------------------------------------------------------------------+
   | Correlation time_cluster_1 (888 series, conf: 0.53,               |
   |   flags: ambiguous_roots)                                          |
   |   Metric families: cpu.user: 45, mem.usage: 38, io.bytes: 31 ... |
   |   NOTE: RCA confidence is low. Focus on metric family breadth.    |
   |   Representative series from most impacted families:              |
   |     - cpu.user (representative of 45 cpu.user series)             |
   |   Instruction: "Do NOT rely on specific rankings for causation."  |
   +--------------------------------------------------------------------+

                                     |
                                     v
                           +-------------------+
                           |    LLM (GPT-5.2)  |
                           |    Diagnosis       |
                           +-------------------+
                                     |
                                     v
                           +-------------------+
                           |  evaluate_        |
                           |  diagnosis.py     |
                           |  Score: 0-100     |
                           +-------------------+
```

## RCA Engine: Scoring

The RCA engine builds an `IncidentGraph` for each active correlation. Every anomalous series becomes a node; temporal proximity between anomalies becomes directed edges (A started before B => A -> B). Each node is scored as a root-cause candidate using six factors:

### Positive Factors (higher = more likely root cause)

| Factor | Weight | Meaning |
|---|---|---|
| **Onset** | 0.35 | How early did this series anomaly relative to others? Earliest = 1.0, latest = 0.0. The intuition: root causes start first. |
| **Coverage** | 0.25 | What fraction of other nodes are downstream (reachable via directed edges)? A node that precedes many others is more likely causal. |
| **Persistence** | 0.15 | How many times did this series anomaly? Normalized vs the most persistent series in the cluster. Sustained anomalies are more significant than one-off blips. |
| **Severity** | 0.15 | How large was the deviation from baseline? `PeakScore / maxPeakScore` in the cluster. A 10-sigma spike matters more than a 2-sigma shift. |

### Negative Factors (higher = less likely root cause)

| Factor | Weight | Meaning |
|---|---|---|
| **Incoming Penalty** | 0.25 | Does another node have a directed edge pointing TO this node? If something else preceded it, this node is likely a symptom, not a root cause. |
| **Spread Penalty** | 0.15 | Is this node's namespace underrepresented in the cluster? If 3/4 of nodes are in the `app` namespace and this node is the sole `cleanup` entry, it's likely a bystander that happened to anomaly at the same time. Only applies when the cluster spans 2+ namespaces. |

### Formula

```
score = onset*0.35 + coverage*0.25 + persistence*0.15 + severity*0.15
        - incomingPenalty*0.25 - spreadPenalty*0.15
```

### Confidence

After scoring, the engine computes an overall confidence (0-1) for the ranking. Three flags can reduce confidence:

| Flag | Trigger | Penalty |
|---|---|---|
| `ambiguous_roots` | Top-2 candidate scores are within 0.05, OR their onset times are within 10% of the cluster window span | -0.20 |
| `weak_directionality` | Less than 30% of edges have an inferred direction | -0.20 |
| `data_limited` | Fewer than 3 nodes, or zero edges in the graph | -0.20 |

The confidence score determines whether the digest uses causal framing (high confidence) or structural summary (low confidence). This gating is critical — without it, low-confidence causal assertions actively mislead the LLM.

## Correlation Digest: Compaction

The digest replaces raw correlation sources with a structured summary. It always includes:

- **`metric_family_counts`**: All sources grouped by metric name with counts (e.g., `{"cpu.user": 45, "mem.usage": 38, ...}`). Gives the LLM a bird's-eye view of which subsystems are affected.
- **`total_source_count`**: Original number of sources before compression.
- **`rca_confidence`** and **`confidence_flags`**: Lets the LLM (and prompt construction) calibrate how much to trust the structural analysis.

When confidence >= 0.6 (high), the digest also includes:
- **`key_sources`**: Top 10 series from the RCA ranking, with scores and explanatory reasons.
- **`onset_chain`**: Up to 8 series sorted by onset time, showing the temporal propagation.

When confidence < 0.6 (low), the digest instead includes:
- **`key_sources`**: 1-2 representative samples from each of the top metric families (sorted by family size). No scores, no causal claims — just examples of what's in each family.
- No onset chain.

This means the digest always compresses (solving the token/size problem) but varies how much causal structure it asserts (solving the misleading-assertion problem).

## Evaluation

### Method

Each scenario is evaluated with the full pipeline:

1. **Observer run**: Process parquet replay through CUSUM + TimeCluster + Dedup, with or without RCA.
2. **LLM diagnosis**: Send JSON output to GPT-5.2 with context about detector/correlator behavior. The prompt includes the digest hint (if present) and asks for: correlations analysis, problem identification, confidence, alternatives, and evidence.
3. **LLM grading**: A separate GPT-5.2 call compares the diagnosis to ground truth and assigns a score (0-100).

### Scenarios

| Scenario | Ground Truth | Difficulty |
|---|---|---|
| **oom-kill** | Python allocates 10MB chunks until 64Mi cgroup limit triggers OOM kill (exit 137) | Medium — resource metrics clearly show memory growth |
| **cpu-starvation** | Backend CPU limits too low for traffic, CFS throttling active, 22% request timeouts | Medium — CPU throttling visible but mixed with other symptoms |
| **traffic-spike** | 18x sudden RPS increase, 48% success rate, CPU saturation across services | Hard — traffic rate not directly visible in container metrics, only side effects |
| **s3-outage** | Operator removes capacity -> etcd quorum loss -> S3 cascade failure | Very hard — root cause is infra-level (etcd), container metrics only see downstream effects |
| **memory-leak** | Python allocates 512KB chunks every 2s, unbounded heap growth until 256Mi OOM kill | Easy — sustained memory growth clearly visible in cgroup metrics |

### Results

Baseline and v3 scores are 5-run averages to reduce LLM grading variance. v1/v2 are single-run (code not preserved for reruns).

| Scenario | Baseline (avg) | v1: Suppress | v2: Digest | v3: Conf-Gate (avg) | Delta |
|---|---|---|---|---|---|
| oom-kill | 79 | 78 | 75 | **78** | -1 |
| cpu-starvation | 74 | 78 | 65 | **71** | -3 |
| traffic-spike | 15 | 15 | 5 | **36** | +21 |
| s3-outage | 8 | 5 | 0 | **4** | -4 |
| memory-leak | 85 | — | — | **79** | -6 |

memory-leak was added after v1/v2 iterations; those cells are blank because the intermediate code was not preserved.

### LLM Payload Sizes

The JSON output is sent directly to the LLM as part of the prompt. The pipeline has no truncation logic — if the payload exceeds the model's context window, the API call fails.

| Scenario | Baseline JSON | RCA JSON | Change | % |
|---|---|---|---|---|
| oom-kill | 20 KB | 22 KB | +2 KB | +10% |
| cpu-starvation | 20 KB | 22 KB | +2 KB | +10% |
| traffic-spike | 55 KB | 35 KB | -20 KB | -36% |
| s3-outage | 1,266 KB† | 44 KB | -1,222 KB | -97% |
| memory-leak | 22 KB | 31 KB | +9 KB | +41% |

†s3-outage baseline is 1,266 KB uncompressed — too large for GPT-5.2's context window (422K tokens vs 272K limit). The pipeline has no truncation logic; the API call simply fails. A manually truncated file (103 KB) was created for evaluation. The RCA digest compresses it to 44 KB automatically — no manual intervention needed.

For small incidents (oom-kill, cpu-starvation, memory-leak), the RCA variant is slightly larger because the digest metadata is additive — the raw sources are small enough to keep, and the digest adds structure on top. For large incidents (traffic-spike, s3-outage), the digest replaces raw sources with a compact summary, producing significant compression.

**v1 (RCA + MinConfidence Suppression):** Added the RCA engine with a MinConfidence=0.5 threshold. Low-confidence results were suppressed entirely from the output. Result: RCA was suppressed in most scenarios, producing output identical to baseline. cpu-starvation got +3 from a scenario that barely cleared the threshold.

**v2 (RCA + Digest, no confidence gate):** Replaced suppression with a correlation digest that compresses sources using RCA analysis. Digest always included RCA-ranked key sources and onset chain regardless of confidence. Compression worked (s3-outage: 1.2MB -> 37KB) but scores regressed everywhere:
- traffic-spike dropped 15 -> 5: digest pointed at a `cleanup` container whose onset was 1 second earlier by coincidence
- s3-outage dropped 15 -> 0: digest pointed at `observer-agent` container instead of the actual etcd issue
- cpu-starvation dropped 75 -> 65: digest narrowed LLM focus away from the broader CPU starvation signal

The root cause: when RCA confidence is low, asserting specific series as root candidates misleads the LLM more than sending raw (or no) data.

**v3 (RCA + Confidence-Gated Digest):** Gate digest content on confidence. High confidence: causal framing with ranked sources and onset chain. Low confidence: structural summary with metric family breadth and representative samples, no causal assertions. All v2 regressions eliminated. Compression preserved.

### Interpretation

**Grading variance is high and asymmetric.** Baseline scores are stable (traffic-spike: 15 across all 5 runs) but RCA scores swing widely (traffic-spike RCA: 8, 15, 15, 65, 75). The digest changes LLM output enough to occasionally trigger different grader interpretations.

**RCA is slightly negative on 4 of 5 scenarios.** oom-kill: -1, cpu-starvation: -3, s3-outage: -4, memory-leak: -6. These are small but consistent. The structural summary in low-confidence mode may slightly constrain the LLM's reasoning compared to raw data.

**traffic-spike is the outlier.** +21 average, but driven by high variance — the digest occasionally prompts the LLM to mention traffic/load as a possibility, earning partial credit. Whether this counts as "lift" or "noise" is ambiguous.

**Compression is the primary value today.** The baseline pipeline sends the full JSON to the LLM with no truncation logic. For s3-outage, the baseline JSON is 1,266 KB (422K tokens) — exceeding GPT-5.2's 272K token context window. The API call simply fails. Evaluation required a manually truncated file (103 KB). The RCA digest compresses it to 44 KB automatically, making large incidents analyzable without manual intervention.

**No positive lift yet.** All five scenarios produce RCA confidence below 0.6, so the high-confidence causal path is untested. The engine correctly identifies that it doesn't have enough directional signal to assert causation — and the confidence gate correctly prevents that uncertain signal from reaching the LLM.

**The hard scenarios remain hard.** traffic-spike (avg 15 baseline) and s3-outage (avg 8 baseline) score low because the ground truths involve phenomena not directly visible in container metrics. Improving these scores likely requires additional data sources (application logs, request rate metrics, infrastructure events) rather than better analysis of the existing container metrics.

## Next Steps

- **Expand test data**
- **Experiment with detection/correlation methods**: Try different detection/correlation algorithms — each produces different graph structures that may improve RCA confidence and directionality.
- **Tune scoring weights**: Empirically optimize the scoring formula weights and confidence threshold against a larger scenario set.
- **Analyze and improve performance**: Profile graph construction (O(n^2) edge building), digest hot paths, and memory footprint under production-scale correlations. Consider incremental graph updates for long-lived correlations and other optimizations. 
