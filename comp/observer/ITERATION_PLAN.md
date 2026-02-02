# Observer: Agentic Iteration Plan

## Goal
Implement **NEW** detection/correlation algorithms (without modifying existing code), test across 10 scenarios, and iterate to improve.

## Constraints
- **DO NOT** modify existing correlators (TimeCluster, GraphSketch)
- **DO NOT** modify existing detectors (CUSUM, LightESD)
- Implement new algorithms as **additions** alongside existing ones
- Use ground truths below for evaluation

---

## Ground Truths (10 Scenarios)

### 1. crash-loop
- **Problem type:** crash loop
- **Fault:** Python script exits with code 1 after random 5-15s delay
- **Mechanism:** sys.exit(1) with restartPolicy: Always triggers Kubernetes restart backoff
- **Root cause:** Intentional application exit(1) causing CrashLoopBackOff
- **Correct keywords:** crash loop, exit code 1, CrashLoopBackOff, restart
- **Partial keywords:** container crash, application failure

### 2. oom-kill
- **Problem type:** OOM kill
- **Fault:** Python script allocates 10MB chunks in loop until killed
- **Mechanism:** Memory limit 64Mi, kernel OOM killer sends SIGKILL
- **Root cause:** Rapid memory allocation exceeds 64Mi cgroup limit, OOM killer terminates (exit code 137)
- **Correct keywords:** OOM, out of memory, exit code 137, OOMKilled
- **Partial keywords:** memory limit, SIGKILL

### 3. sigpipe-crash
- **Problem type:** SIGPIPE crash
- **Fault:** uds-server exits every 30s, victim-app writes to closed socket
- **Mechanism:** C library doesn't handle SIGPIPE, process killed with signal 13
- **Root cause:** Broken Unix Domain Socket pipe - uds-server exits (code 0), victim-app gets SIGPIPE (exit code 141 = 128+13)
- **Correct keywords:** SIGPIPE, exit code 141, broken pipe, signal 13
- **Partial keywords:** socket, pipe error

### 4. todo-app-redis-backend-cpu-starvation
- **Problem type:** CPU starvation
- **Fault:** Backend CPU limits too low for traffic, Kubernetes throttles pods
- **Mechanism:** CPU at 100%, CFS throttling active, 22% requests timeout
- **Root cause:** CPU limits insufficient causing Kubernetes CFS throttling - 78% slow (8-12x), 22% timeout after 30s
- **Correct keywords:** CPU starvation, CPU throttling, throttled, CPU limit
- **Partial keywords:** CPU saturation, slow responses

### 5. todo-app-redis-connection-timeout
- **Problem type:** connection timeout
- **Fault:** Network partition between backend and Redis pods
- **Mechanism:** 80% of Redis operations timeout after 5 seconds
- **Root cause:** Network partition causes 80% Redis connection timeouts (5s), 20% succeed with ~4200ms latency
- **Correct keywords:** connection timeout, network partition, Redis timeout, i/o timeout
- **Partial keywords:** Redis unavailable, network issue

### 6. todo-app-redis-deployment-v2-slow-serialization
- **Problem type:** slow serialization
- **Fault:** Version v2.0.5 switched to reflection-based JSON marshaling
- **Mechanism:** 3x serialization overhead, 12% panic rate
- **Root cause:** Deployment v2.0.5 regression - reflection-based JSON marshaling causes 3-4x latency and 12% serialization panics
- **Correct keywords:** serialization, JSON, v2.0.5, deployment regression
- **Partial keywords:** latency regression, new version

### 7. todo-app-redis-memory-exhaustion
- **Problem type:** memory exhaustion
- **Fault:** Redis maxmemory limit (256Mi) reached
- **Mechanism:** Eviction policy can't keep pace, 70% writes fail with OOM
- **Root cause:** Redis maxmemory exceeded - 70% writes fail with 'OOM command not allowed', 30% succeed after evictions
- **Correct keywords:** memory exhaustion, Redis OOM, maxmemory, eviction
- **Partial keywords:** Redis memory, write failures

### 8. todo-app-redis-traffic-spike
- **Problem type:** traffic spike
- **Fault:** 18x sudden increase in requests per second
- **Mechanism:** All services saturated, only 48% success rate
- **Root cause:** 18x RPS spike overwhelming system - backend/Redis at 100% CPU, 48% success, 42% overload errors, 10% timeout
- **Correct keywords:** traffic spike, 18x, overloaded, traffic surge
- **Partial keywords:** high load, saturation

### 9. memory-leak (existing)
- **Problem type:** memory leak
- **Root cause:** Gradual memory leak in Python app allocating 512KB chunks every 2s until OOM at 256Mi

### 10. todo-app-redis-network-high-latency (existing)
- **Problem type:** network latency
- **Root cause:** Artificial 200ms ± 50ms network latency on Redis pod using tc netem

---

## Current State

**Existing Infrastructure:**
- `harness.py`: Bayesian optimization with Optuna
- `analyze_with_llm.py`: GPT diagnosis generation
- `evaluate_diagnosis.py`: GPT grading (0-100)
- `observer-demo-v2`: Binary that runs detectors + correlators on parquet files

**Existing Correlators (DO NOT MODIFY):**
- TimeClusterCorrelator (temporal proximity)
- GraphSketchCorrelator (co-occurrence patterns)

**Existing Detectors (DO NOT MODIFY):**
- CUSUM (change-point detection)
- LightESD (statistical outliers)
- GraphSketch emitter (edge frequency)

**Current Performance:**
- Memory leak: 80/100 (CUSUM + TimeCluster)
- Network latency: <10/100 (all combinations fail)
- Other 8 scenarios: Not yet tested

---

## Agentic Iteration Strategy

### Train/Test Split (Avoid Overfitting)

**TRAIN SET (6 scenarios) - Develop and tune on these:**
1. memory-leak.parquet
2. crash-loop.parquet
3. todo-app-redis-connection-timeout.parquet
4. todo-app-redis-memory-exhaustion.parquet
5. todo-app-redis-traffic-spike.parquet
6. todo-app-redis-network-high-latency.parquet

**TEST SET (4 scenarios) - Final evaluation only:**
1. oom-kill.parquet
2. sigpipe-crash.parquet
3. todo-app-redis-backend-cpu-starvation.parquet
4. todo-app-redis-deployment-v2-slow-serialization.parquet

### Iteration Loop

```
┌─────────────────────────────────────────────────────────────────┐
│                     AGENTIC ITERATION LOOP                      │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  STEP 1: IMPLEMENT                                              │
│    └── Write new correlator/detector in comp/observer/impl/     │
│    └── Add CLI flags to cmd/observer-demo-v2/main.go            │
│    └── Wire up in demo_main_v2.go                               │
│                                                                 │
│  STEP 2: BUILD                                                  │
│    └── go build -o bin/observer-demo-v2 ./cmd/observer-demo-v2  │
│                                                                 │
│  STEP 3: TEST ON ONE SCENARIO                                   │
│    └── ./bin/observer-demo-v2 --parquet <train_scenario> \      │
│           --output test.json --cusum --<new-correlator> --all   │
│    └── python3 analyze_with_llm.py test.json                    │
│    └── python3 evaluate_diagnosis.py diagnosis.txt \            │
│           --scenario <scenario>                                 │
│                                                                 │
│  STEP 4: ANALYZE FAILURE                                        │
│    └── If score < 50: Read diagnosis, identify missing evidence │
│    └── Check: What signals exist? What correlations found?      │
│    └── Determine: What evidence would help LLM diagnose?        │
│                                                                 │
│  STEP 5: IMPROVE ALGORITHM                                      │
│    └── Adjust algorithm logic based on failure analysis         │
│    └── Go back to STEP 1                                        │
│                                                                 │
│  STEP 6: CROSS-VALIDATE (when single scenario works)            │
│    └── Test on ALL 6 train scenarios                            │
│    └── Compute average score                                    │
│    └── If avg < 50: identify failing scenarios, go to STEP 4    │
│                                                                 │
│  STEP 7: FINAL EVAL (when train avg > 60)                       │
│    └── Test on 4 held-out TEST scenarios                        │
│    └── Report generalization performance                        │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## Phase 0: Add Ground Truths to evaluate_diagnosis.py

Add the 8 new scenarios to `evaluate_diagnosis.py`:

```python
PROBLEM_TYPES = {
    "memory-leak": "memory leak",
    "network-latency": "network latency",
    # ADD THESE:
    "crash-loop": "crash loop",
    "oom-kill": "OOM kill",
    "sigpipe-crash": "SIGPIPE crash",
    "cpu-starvation": "CPU starvation",
    "connection-timeout": "connection timeout",
    "slow-serialization": "slow serialization",
    "memory-exhaustion": "memory exhaustion",
    "traffic-spike": "traffic spike",
}
```

And corresponding GROUND_TRUTHS entries with the details from above.

---

## Phase 1: Implement New Algorithms

### 1.1 Stable Bloom Filter Deduplication (Pre-Correlation Layer)
**File:** `comp/observer/impl/anomaly_dedup.go`

Custom implementation (no external libs). Reduces anomaly volume before correlation.

```go
type StableBloomFilter struct {
    cells     []uint8   // Counter array
    numCells  uint32
    numHashes uint32
    max       uint8     // Max counter value
    p         uint32    // Cells to decrement per add (eviction rate)
}

type AnomalyDeduplicator struct {
    filter            *StableBloomFilter
    bucketSizeSeconds int64
}

func (d *AnomalyDeduplicator) ShouldProcess(source string, timestamp int64) bool
```

**CLI Flags:**e
- `--dedup`: Enable deduplication
- `--dedup-bucket-seconds`: Time bucket size (default: 5)

### 1.2 Lead-Lag Correlator (NEW)
**File:** `comp/observer/impl/anomaly_processor_leadlag.go`

Detects "A leads B by N seconds" patterns for root cause identification.

```go
type LeadLagCorrelator struct {
    sourceTimestamps map[string]*RingBuffer   // Recent timestamps per source
    lagHistograms    map[string]*LagHistogram // Pair lag distributions
    maxLagSeconds    int64
    minObservations  int
}

type LeadLagEdge struct {
    Leader       string
    Follower     string
    TypicalLag   int64
    Confidence   float64
    Observations int
}
```

**CLI Flags:**
- `--lead-lag`: Enable lead-lag correlator
- `--leadlag-max-lag`: Max lag to track (default: 30s)
- `--leadlag-min-obs`: Min observations for edge (default: 3)

### 1.3 Surprise/Lift Correlator (NEW)
**File:** `comp/observer/impl/anomaly_processor_surprise.go`

Detects unexpected co-occurrences using lift (PMI-like metric).

```go
type SurpriseCorrelator struct {
    sourceCounts         map[string]int    // Marginal counts
    pairCounts           map[string]int    // Joint counts
    totalWindows         int
    windowSizeSeconds    int64
    currentWindowSources map[string]bool
}

// lift(A,B) = P(A∩B) / (P(A) × P(B))
// lift > 2: surprisingly together
// lift < 0.5: surprisingly apart
```

**CLI Flags:**
- `--surprise`: Enable surprise correlator
- `--surprise-window`: Window size (default: 10s)
- `--surprise-min-lift`: Min lift threshold (default: 2.0)

---

## Phase 2: Wire Up New Algorithms

### 2.1 Add to cmd/observer-demo-v2/main.go

```go
// New correlator flags (add alongside existing)
leadLag := flag.Bool("lead-lag", false, "use LeadLagCorrelator")
surprise := flag.Bool("surprise", false, "use SurpriseCorrelator")
dedup := flag.Bool("dedup", false, "enable deduplication layer")

// Tuning parameters
leadLagMaxLag := flag.Int64("leadlag-max-lag", 0, "max lag seconds (default: 30)")
leadLagMinObs := flag.Int("leadlag-min-obs", 0, "min observations (default: 3)")
surpriseWindow := flag.Int64("surprise-window", 0, "window size seconds (default: 10)")
surpriseMinLift := flag.Float64("surprise-min-lift", 0, "min lift threshold (default: 2.0)")
dedupBucket := flag.Int64("dedup-bucket", 0, "bucket size seconds (default: 5)")
```

### 2.2 Add to demo_main_v2.go DemoV2Config

```go
type DemoV2Config struct {
    // ... existing fields ...

    // NEW correlators
    EnableLeadLagCorrelator  bool
    EnableSurpriseCorrelator bool
    EnableDedup              bool

    // NEW tuning params
    LeadLagMaxLag    int64
    LeadLagMinObs    int
    SurpriseWindow   int64
    SurpriseMinLift  float64
    DedupBucket      int64
}
```

### 2.3 Instantiate in RunDemoV2WithConfig

```go
// After existing correlator selection, add:
if config.EnableLeadLagCorrelator {
    llConfig := DefaultLeadLagConfig()
    if config.LeadLagMaxLag > 0 {
        llConfig.MaxLagSeconds = config.LeadLagMaxLag
    }
    // ... apply other overrides
    correlator = NewLeadLagCorrelator(llConfig)
}

if config.EnableSurpriseCorrelator {
    sConfig := DefaultSurpriseConfig()
    // ... apply overrides
    correlator = NewSurpriseCorrelator(sConfig)
}

// Dedup wraps the correlator
if config.EnableDedup {
    dedup := NewAnomalyDeduplicator(config.DedupBucket)
    // Integrate with anomaly processing pipeline
}
```

---

## Phase 3: Agentic Iteration Commands

### Single Scenario Test (Quick Feedback)
```bash
# Build
go build -o bin/observer-demo-v2 ./cmd/observer-demo-v2

# Run with new correlator
./bin/observer-demo-v2 \
    --parquet q_branch/fine-grained-monitor/anomaly_datasets/crash-loop.parquet \
    --output /tmp/test.json \
    --cusum \
    --lead-lag \
    --all

# Diagnose
python3 analyze_with_llm.py /tmp/test.json --cusum --leadlag > /tmp/diagnosis.txt

# Grade
python3 evaluate_diagnosis.py /tmp/diagnosis.txt --scenario crash-loop
```

### Multi-Scenario Evaluation
```bash
# Test across all TRAIN scenarios
for scenario in memory-leak crash-loop connection-timeout memory-exhaustion traffic-spike network-latency; do
    parquet="q_branch/fine-grained-monitor/anomaly_datasets/${scenario}.parquet"
    # Map scenario names to parquet files as needed
    ./bin/observer-demo-v2 --parquet "$parquet" --output "/tmp/${scenario}.json" --cusum --lead-lag --all
    python3 analyze_with_llm.py "/tmp/${scenario}.json" > "/tmp/${scenario}_diag.txt"
    echo "=== $scenario ===" >> results.txt
    python3 evaluate_diagnosis.py "/tmp/${scenario}_diag.txt" --scenario "$scenario" >> results.txt
done
```

### Failure Analysis
When a scenario scores low:
1. Read the observer output JSON - what anomalies were detected?
2. Read the correlations - what patterns were found?
3. Read the LLM diagnosis - what did it conclude?
4. Identify gap: What evidence would help the LLM diagnose correctly?

---

## Phase 4: Success Criteria

### Per-Scenario Targets (Train Set)
| Scenario | Target Score |
|----------|-------------|
| memory-leak | >70 |
| crash-loop | >50 |
| connection-timeout | >50 |
| memory-exhaustion | >50 |
| traffic-spike | >50 |
| network-latency | >40 |

### Generalization (Test Set - Final Only)
| Scenario | Target Score |
|----------|-------------|
| oom-kill | >40 |
| sigpipe-crash | >40 |
| cpu-starvation | >40 |
| slow-serialization | >40 |

### Algorithm Comparison
Compare new correlators vs existing:
- Lead-Lag vs TimeCluster
- Surprise vs GraphSketch
- Combined (Lead-Lag + Surprise + Dedup) vs best existing

---

## Files Summary

### Files to CREATE (New Code)
| File | Purpose |
|------|---------|
| `comp/observer/impl/anomaly_dedup.go` | Stable Bloom Filter deduplication |
| `comp/observer/impl/anomaly_processor_leadlag.go` | Lead-Lag correlator |
| `comp/observer/impl/anomaly_processor_surprise.go` | Surprise/Lift correlator |

### Files to MODIFY (Integration)
| File | Changes |
|------|---------|
| `cmd/observer-demo-v2/main.go` | Add CLI flags for new correlators |
| `comp/observer/impl/demo_main_v2.go` | Add config fields, instantiate new correlators |
| `evaluate_diagnosis.py` | Add 8 new ground truth definitions |

### Files NOT to Touch
- `comp/observer/impl/anomaly_processor_time_cluster.go`
- `comp/observer/impl/anomaly_processor_graphsketch.go`
- `comp/observer/impl/emitter_lightesd.go`
- `comp/observer/impl/emitter_graphsketch.go`
- `comp/observer/impl/ts_analysis_cusum.go`

---

## Quick Start Commands

### Build & Test Single Scenario
```bash
# Build
go build -o bin/observer-demo-v2 ./cmd/observer-demo-v2

# Test with new correlator on crash-loop
./bin/observer-demo-v2 \
    --parquet q_branch/fine-grained-monitor/anomaly_datasets/crash-loop.parquet \
    --output /tmp/crash-loop.json \
    --cusum \
    --lead-lag \
    --all

# Get diagnosis
python3 analyze_with_llm.py /tmp/crash-loop.json --cusum > /tmp/diagnosis.txt

# Score it
python3 evaluate_diagnosis.py /tmp/diagnosis.txt --scenario crash-loop
```

### Parquet File Mapping
```
crash-loop.parquet                              → --scenario crash-loop
oom-kill.parquet                                → --scenario oom-kill
sigpipe-crash.parquet                           → --scenario sigpipe-crash
memory-leak.parquet                             → --scenario memory-leak
todo-app-redis-backend-cpu-starvation.parquet   → --scenario cpu-starvation
todo-app-redis-connection-timeout.parquet       → --scenario connection-timeout
todo-app-redis-deployment-v2-slow-serialization.parquet → --scenario slow-serialization
todo-app-redis-memory-exhaustion.parquet        → --scenario memory-exhaustion
todo-app-redis-network-high-latency.parquet     → --scenario network-latency
todo-app-redis-traffic-spike.parquet            → --scenario traffic-spike
```

---

## Experimental Best Practices

### Evaluation Matrix Rules

1. **Run complete evaluation matrices** - Always test all configs on all scenarios. Never leave cells empty in the matrix.

2. **No selective/sparse evaluation** - Don't skip running a config because it "seems unlikely to work." Run it anyway to get the data.

3. **Ask before skipping** - If something truly seems fruitless (e.g., data-limited scenarios consistently scoring <10), raise the concern and get explicit permission before omitting from evaluation.

4. **Document all results** - Even negative results (low scores) are valuable data. Record everything.

5. **Tuning must be exhaustive** - If tuning one config, tune all configs. Sparse tuning leads to misleading conclusions about relative performance.

### Data Integrity

6. **Test set is sacred** - Never tune on test set. Only use it for final evaluation after all development is complete.

7. **Train/test split is fixed** - Don't cherry-pick scenarios between sets based on results.

8. **Preserve all artifacts** - Keep JSON outputs, diagnosis files, and tuning logs for reproducibility.

### Communication

9. **Report uncertainty** - If a result seems anomalous (e.g., score dropped 90 points after minor change), flag it and investigate before moving on.

10. **No silent omissions** - If you can't run something (missing data, build error, etc.), report it explicitly rather than leaving it out of results.
