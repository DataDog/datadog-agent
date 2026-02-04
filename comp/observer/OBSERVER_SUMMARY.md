# Observer: Anomaly Detection & Correlation

Surface correlated anomalies from time-series data to help an LLM diagnose root causes.

---

## Architecture

```
Raw Metrics (1M+ data points)
       │
       ▼
┌──────────────────────────────────────┐
│  Layer 1: CUSUM Detector             │
│  Change-point detection              │
│  Filters :count metrics (noise)      │
│  Output: ~50K anomalies              │
└──────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────┐
│  Layer 1.5: Deduplication            │
│  Stable Bloom Filter                 │
│  Output: ~700 anomalies (98.7%↓)     │
└──────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────┐
│  Layer 2: Correlator                 │
│                                      │
│  TimeCluster │ LeadLag  │ Surprise   │
│  (proximity) │ (causal) │ (lift)     │
└──────────────────────────────────────┘
       │
       ▼
   LLM Diagnosis
```

### Layer 1: CUSUM Detector
**Cumulative Sum (CUSUM)** detects abrupt shifts in metric values by tracking cumulative deviations from a baseline mean. When the cumulative sum exceeds a threshold, it signals a change-point.

Critical: filters `:count` metrics (container cardinality counts that create 131 noise sources showing identical "1→2" scaling patterns).

### Layer 1.5: Deduplication
**Stable Bloom Filter** handles unbounded streams by probabilistically evicting old entries. Key: `source + timestamp/5s` bucketing reduces 50K anomalies to 700 (98.7% reduction) while preserving unique signals.

### Layer 2: Correlators

#### TimeCluster (Temporal Proximity)
Groups anomalies that occur within N seconds of each other into clusters.

```
If anomaly A at t=10s and anomaly B at t=12s (within 5s window):
  → Cluster {A, B} - "these happened together"
```

**Output:** "Cluster of 9 anomalies: memory, CPU, latency all shifted simultaneously"
**Best for:** Sudden failures where multiple metrics spike at once (crash-loop, OOM)

#### LeadLag (Causal Chains)
Tracks temporal ordering to detect "A leads B by N seconds" patterns. Maintains lag histograms for each source pair.

```
If network.retransmits spikes, then 5s later connection.errors spikes:
  → "network.retransmits leads connection.errors by ~5s"
```

**Output:** "Temporal: memory.usage leads app.latency by ~10s (73% confidence)"
**Best for:** Cascading failures where root cause propagates (memory-leak → GC pause → latency)

#### Surprise (Lift-based Co-occurrence)
Detects unexpected co-occurrences using lift from association rule mining:

```
lift(A,B) = P(A ∩ B) / (P(A) × P(B))
  - lift > 2.0: A and B co-occur MORE than expected (surprising)
  - lift < 0.5: A and B co-occur LESS than expected (also interesting)
  - lift ≈ 1.0: Independent, random co-occurrence
```

**Output:** "Surprising: disk.io_wait + network.retransmits co-occur (lift=3.8)"
**Best for:** Finding unusual patterns that wouldn't be expected by chance

---

## Development Process

Used an agentic iteration loop:

1. **Run** observer on scenario
2. **Diagnose** with LLM
3. **Grade** against ground truth (0-100)
4. **Analyze** failures - what signals are missing?
5. **Improve** algorithm and repeat

Split scenarios into train (6) and test (4) sets to avoid overfitting.

See **ITERATION_RECAP.md** for detailed run-by-run findings.

---

## Results

### Complete Evaluation Matrix (Train Set)

| Scenario | TC | TC+D | TC+D+T | LL | LL+D | LL+D+T | SP | SP+D | SP+D+T | Best |
|----------|-----|------|--------|-----|------|--------|-----|------|--------|------|
| memory-leak | 78 | 92 | **98** | 85 | 95 | 95 | 78 | 82 | 92 | **98** (TC+D+T) |
| crash-loop | 92 | 85 | 85 | 2 | 75 | **95** | 78 | 78 | **95** | **95** (LL+D+T/SP+D+T) |
| memory-exhaustion | 15 | 15 | **78** | 75 | 75 | **78** | 5 | 45 | 40 | **78** (TC+D+T/LL+D+T) |
| traffic-spike | 65 | 35 | **78** | 5 | 65 | 15 | 25 | 35 | 20 | **78** (TC+D+T) |
| connection-timeout | 8 | 5 | **15** | 3 | 5 | **15** | 5 | 5 | 5 | **15** (TC+D+T/LL+D+T) |
| network-latency | **15** | 15 | 15 | 5 | 5 | 15 | 2 | 2 | 15 | **15** (multiple) |
| **Average** | 45.5 | 41.2 | **61.5** | 29.2 | 53.3 | 52.2 | 32.2 | 41.2 | 44.5 | **64.7** |

**Legend:** TC=TimeCluster, LL=LeadLag, SP=Surprise, +D=Dedup, +T=Tuned (Optuna, 5 trials)

### Test Set Results (Held-Out Scenarios)

| Scenario | TC | TC+D | LL+D | SP+D | TC+D+T | LL+D+T | SP+D+T | Best |
|----------|-----|------|------|------|--------|--------|--------|------|
| oom-kill | **97** | 95 | 82 | 78 | 85 | 95 | 85 | **97** (TC) |
| sigpipe-crash | 2 | **5** | 3 | 2 | **5** | **5** | **5** | **5** (multiple) |
| cpu-starvation | 20 | **78** | **78** | 20 | 20 | 35 | 0 | **78** (TC+D/LL+D) |
| slow-serialization | 3 | **12** | 2 | **12** | 5 | 5 | 3 | **12** (TC+D/SP+D) |
| **Average** | 30.5 | **47.5** | 41.3 | 28.0 | 28.8 | 35.0 | 23.3 | **48.0** |

**Legend:** TC=TimeCluster, LL=LeadLag, SP=Surprise, +D=Dedup, +T=Tuned

### Summary

| Set | Best Single Config | Best Possible (per-scenario) |
|-----|-------------------|------------------------------|
| Train (6 scenarios) | TC+D+T: 61.5 | 64.7 |
| Test (4 scenarios) | TC+D: 47.5 | 48.0 |

**Key observations:**
- **Dedup helps on both train and test**: TC+D (47.5) outperforms TC (30.5) on test by +17 points
- **Tuning helps on train, hurts on test**: TC+D+T (61.5) on train, but (28.8) on test vs TC+D (47.5)
- **cpu-starvation: dedup is critical**: TC (20) vs TC+D (78) - a +58 point improvement
- **2 scenarios are data-limited**: sigpipe-crash, slow-serialization score ≤12

### Key Findings

1. **Deduplication is critical for both train and test** - TC+D (47.5) outperforms TC (30.5) on test set
2. **Dedup transforms cpu-starvation** - TC (20) vs TC+D (78), a +58 point improvement
3. **Tuning helps train, hurts test** - TC+D+T (61.5 train, 28.8 test) vs TC+D (41.2 train, 47.5 test)
4. **Data quality matters** - sigpipe-crash (max 5) and slow-serialization (max 12) are data-limited
5. **TC+D is best generalizing config** - 47.5 avg on test, consistent across diverse failure types
6. **LeadLag + Dedup excels at cascading failures** - memory-leak (95), but not as versatile as TC+D

---

## Recommendations

**Train set best:** TC+D+T (61.5 avg) - but tuning is scenario-specific
**Test set best:** TC+D (47.5 avg) - dedup helps generalization

**Default:** `--cusum --time-cluster --dedup` (best generalization to unseen scenarios)

| Suspected Issue | Use |
|-----------------|-----|
| Memory leak | LeadLag + Dedup |
| Crash loop / OOM | TimeCluster + Dedup |
| CPU starvation | TimeCluster + Dedup |
| Traffic spike | TimeCluster + Dedup |
| Unknown | Try TimeCluster + Dedup first |

---

## Files

| File | Purpose |
|------|---------|
| `ITERATION_RECAP.md` | Run-by-run development history |
| `ITERATION_PLAN.md` | Original plan and ground truths |
| `tuning/harness.py` | Optuna tuning harness |
| `tuning/results/` | All evaluation results |
