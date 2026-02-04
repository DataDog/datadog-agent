# Observer: Example Implementation Context

This document contains specific algorithm implementations and configurations used in the observer evaluation experiments. Use this as reference context when working with the evaluation framework.

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

### 4. cpu-starvation
- **Problem type:** CPU starvation
- **Fault:** Backend CPU limits too low for traffic, Kubernetes throttles pods
- **Mechanism:** CPU at 100%, CFS throttling active, 22% requests timeout
- **Root cause:** CPU limits insufficient causing Kubernetes CFS throttling - 78% slow (8-12x), 22% timeout after 30s
- **Correct keywords:** CPU starvation, CPU throttling, throttled, CPU limit
- **Partial keywords:** CPU saturation, slow responses

### 5. connection-timeout
- **Problem type:** connection timeout
- **Fault:** Network partition between backend and Redis pods
- **Mechanism:** 80% of Redis operations timeout after 5 seconds
- **Root cause:** Network partition causes 80% Redis connection timeouts (5s), 20% succeed with ~4200ms latency
- **Correct keywords:** connection timeout, network partition, Redis timeout, i/o timeout
- **Partial keywords:** Redis unavailable, network issue

### 6. slow-serialization
- **Problem type:** slow serialization
- **Fault:** Version v2.0.5 switched to reflection-based JSON marshaling
- **Mechanism:** 3x serialization overhead, 12% panic rate
- **Root cause:** Deployment v2.0.5 regression - reflection-based JSON marshaling causes 3-4x latency and 12% serialization panics
- **Correct keywords:** serialization, JSON, v2.0.5, deployment regression
- **Partial keywords:** latency regression, new version

### 7. memory-exhaustion
- **Problem type:** memory exhaustion
- **Fault:** Redis maxmemory limit (256Mi) reached
- **Mechanism:** Eviction policy can't keep pace, 70% writes fail with OOM
- **Root cause:** Redis maxmemory exceeded - 70% writes fail with 'OOM command not allowed', 30% succeed after evictions
- **Correct keywords:** memory exhaustion, Redis OOM, maxmemory, eviction
- **Partial keywords:** Redis memory, write failures

### 8. traffic-spike
- **Problem type:** traffic spike
- **Fault:** 18x sudden increase in requests per second
- **Mechanism:** All services saturated, only 48% success rate
- **Root cause:** 18x RPS spike overwhelming system - backend/Redis at 100% CPU, 48% success, 42% overload errors, 10% timeout
- **Correct keywords:** traffic spike, 18x, overloaded, traffic surge
- **Partial keywords:** high load, saturation

### 9. memory-leak
- **Problem type:** memory leak
- **Root cause:** Gradual memory leak in Python app allocating 512KB chunks every 2s until OOM at 256Mi

### 10. network-latency
- **Problem type:** network latency
- **Root cause:** Artificial 200ms +/- 50ms network latency on Redis pod using tc netem

---

## Train/Test Split

**TRAIN SET (6 scenarios):**
1. memory-leak
2. crash-loop
3. connection-timeout
4. memory-exhaustion
5. traffic-spike
6. network-latency

**TEST SET (4 scenarios):**
1. oom-kill
2. sigpipe-crash
3. cpu-starvation
4. slow-serialization

---

## Implemented Algorithms

### Detectors (Layer 1)

#### CUSUM
Change-point detection using cumulative sum statistics.

**Parameters:**
| Parameter | Default | Range | Notes |
|-----------|---------|-------|-------|
| baseline_fraction | 0.25 | [0.1, 0.5] | Fraction of data for baseline estimation |
| slack_factor | 0.5 | [0.1, 2.0] | Multiplier for stddev -> slack k |
| threshold_factor | 4.0 | [2.0, 8.0] | Multiplier for stddev -> threshold h |

**Files:** `comp/observer/impl/ts_analysis_cusum.go`

#### LightESD
Statistical outlier detection with seasonality handling.

**Parameters:**
| Parameter | Default | Range | Notes |
|-----------|---------|-------|-------|
| min_window_size | 50 | [20, 200] | Min points for analysis |
| alpha | 0.05 | [0.001, 0.1] | Significance level (lower = stricter) |
| trend_window_fraction | 0.15 | [0.05, 0.3] | Fraction for trend smoothing |
| periodicity_significance | 0.01 | [0.001, 0.1] | p-value for seasonality detection |
| max_periods | 2 | [1, 4] | Max seasonal components |

**Files:** `comp/observer/impl/emitter_lightesd.go`

### Correlators (Layer 2)

#### TimeCluster
Groups anomalies by temporal proximity.

**Parameters:**
| Parameter | Default | Range | Notes |
|-----------|---------|-------|-------|
| slack_seconds | 1 | [1, 30] | Time slack for grouping |

**Files:** `comp/observer/impl/anomaly_processor_time_cluster.go`

#### LeadLag
Detects "A leads B by N seconds" patterns for root cause identification.

**Parameters:**
| Parameter | Default | Range | Notes |
|-----------|---------|-------|-------|
| max_lag_seconds | 30 | [5, 60] | Maximum lag to track |
| min_observations | 3 | [2, 10] | Minimum observations for edge |

**Files:** `comp/observer/impl/anomaly_processor_leadlag.go`

#### Surprise
Detects unexpected co-occurrences using lift (PMI-like metric).

**Parameters:**
| Parameter | Default | Range | Notes |
|-----------|---------|-------|-------|
| window_size_seconds | 10 | [5, 60] | Window size for co-occurrence |
| min_lift | 2.0 | [1.5, 5.0] | Minimum lift threshold |

**Files:** `comp/observer/impl/anomaly_processor_surprise.go`

#### GraphSketch
Co-occurrence learning with decay.

**Parameters:**
| Parameter | Default | Range | Notes |
|-----------|---------|-------|-------|
| cooccurrence_window | 10 | [1, 60] | Seconds to consider co-occurring |
| decay_factor | 0.85 | [0.5, 0.99] | Time decay (higher = slower decay) |
| min_correlation_strength | 2.0 | [0.5, 10.0] | Min edge frequency to cluster |
| edge_limit | 200 | [50, 1000] | Top-K edges to track |

**Files:** `comp/observer/impl/anomaly_processor_graphsketch.go`

### Pre-processing

#### Stable Bloom Filter Deduplication
Reduces anomaly volume using probabilistic duplicate detection.

**Parameters:**
| Parameter | Default | Range | Notes |
|-----------|---------|-------|-------|
| num_cells | 100000 | - | Filter size (~100KB memory) |
| num_hashes | 3 | - | Number of hash functions |
| max_counter | 3 | - | Counter ceiling |
| p | 1 | - | Cells to decrement per add |
| bucket_size_seconds | 5 | [1, 30] | Time bucket for dedup key |

**Files:** `comp/observer/impl/anomaly_dedup.go`

---

## CLI Flags

```bash
# Detectors
--cusum          # Enable CUSUM detector
--lightesd       # Enable LightESD detector

# Correlators
--time-cluster   # Enable TimeCluster correlator
--lead-lag       # Enable LeadLag correlator
--surprise       # Enable Surprise correlator
--graphsketch    # Enable GraphSketch correlator

# Pre-processing
--dedup          # Enable Stable Bloom Filter deduplication

# Tuning overrides
--cusum-baseline-fraction=0.25
--cusum-slack-factor=0.5
--cusum-threshold-factor=4.0
--timecluster-slack-seconds=1
--leadlag-max-lag=30
--leadlag-min-obs=3
--surprise-window=10
--surprise-min-lift=2.0
--dedup-bucket-seconds=5
```

---

## Evaluation Results Summary

### Best Configuration

**Default recommendation:** `--cusum --time-cluster --dedup`

| Metric | Train Set | Test Set |
|--------|-----------|----------|
| TC+D avg | 52.3 | 47.5 |
| TC+D+T (tuned) avg | 61.5 | 28.8 |

**Key findings:**
- Deduplication is critical: TC+D (47.5) vs TC (30.5) on test set
- Tuning helps train but hurts test: overfitting risk
- TimeCluster outperforms other correlators on average

### Per-Scenario Test Results (TC+D)

| Scenario | Score |
|----------|-------|
| oom-kill | 95 |
| sigpipe-crash | 5 |
| cpu-starvation | 78 |
| slow-serialization | 12 |
| **Average** | **47.5** |

---

## Parquet File Mapping

```
crash-loop.parquet                              -> --scenario crash-loop
oom-kill.parquet                                -> --scenario oom-kill
sigpipe-crash.parquet                           -> --scenario sigpipe-crash
memory-leak.parquet                             -> --scenario memory-leak
todo-app-redis-backend-cpu-starvation.parquet   -> --scenario cpu-starvation
todo-app-redis-connection-timeout.parquet       -> --scenario connection-timeout
todo-app-redis-deployment-v2-slow-serialization.parquet -> --scenario slow-serialization
todo-app-redis-memory-exhaustion.parquet        -> --scenario memory-exhaustion
todo-app-redis-network-high-latency.parquet     -> --scenario network-latency
todo-app-redis-traffic-spike.parquet            -> --scenario traffic-spike
```
