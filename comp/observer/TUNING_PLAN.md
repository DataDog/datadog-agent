# Observer Anomaly Detection Tuning Plan

## Goal

Surface interesting correlated anomalies from timeseries data. Improve diagnosis scores on memory leak and network latency scenarios.

**Current Results:**
- Memory leak: CUSUM + TimeCluster = 80/100 (best)
- Network latency: All fail (<10/100)

**Known Issue:** GraphSketch backpressure drops metrics, making results unreliable.

---

## Step 1: Fix GraphSketch Bottleneck

**Problem:** GraphSketch is slow, causing backpressure that drops metrics.
- Memory leak: 23,871 anomalies (GraphSketch) vs 82,064 (TimeCluster)
- Same detector, vastly different anomaly counts = unreliable comparison

**Root Cause:** `getLearnedEdgesLocked()` (line 571-618) sorts all edges and calculates decay-weighted frequency for top 200 on every call.

**Fix:** Cache the expensive computation.

```go
// anomaly_processor_graphsketch.go - add to struct
type GraphSketchCorrelator struct {
    // ... existing fields

    // Caching
    cachedTopEdges      []EdgeInfo
    cacheValidUntil     time.Time
    cacheTTL            time.Duration  // Default: 5s
}

// Replace GetLearnedEdges()
func (g *GraphSketchCorrelator) GetLearnedEdges() []EdgeInfo {
    g.mu.RLock()
    if time.Now().Before(g.cacheValidUntil) {
        edges := g.cachedTopEdges
        g.mu.RUnlock()
        return edges
    }
    g.mu.RUnlock()

    g.mu.Lock()
    defer g.mu.Unlock()

    // Double-check after acquiring write lock
    if time.Now().Before(g.cacheValidUntil) {
        return g.cachedTopEdges
    }

    g.cachedTopEdges = g.getLearnedEdgesLocked()
    g.cacheValidUntil = time.Now().Add(g.cacheTTL)
    return g.cachedTopEdges
}
```

**Validation:** Re-run parquet replay, confirm anomaly counts match between correlators.

---

## Step 2: Build Tuning Harness

**Cycle:** tune params → run replay → GPT-5.2 diagnosis → GPT-5.2 score

### 2.1 Add Config Flags to Demo

Modify `demo_main_v2.go` to accept parameter overrides:

```go
// Config struct for tunable parameters
type TuningConfig struct {
    // CUSUM
    CUSUMBaselineFraction float64  // Default: 0.25
    CUSUMSlackFactor      float64  // Default: 0.5
    CUSUMThresholdFactor  float64  // Default: 4.0

    // LightESD
    LightESDMinWindowSize          int     // Default: 50
    LightESDAlpha                  float64 // Default: 0.05
    LightESDTrendWindowFraction    float64 // Default: 0.15
    LightESDPeriodicitySignificance float64 // Default: 0.01
    LightESDMaxPeriods             int     // Default: 2

    // GraphSketch
    GraphSketchCoOccurrenceWindow int64   // Default: 10
    GraphSketchDecayFactor        float64 // Default: 0.85
    GraphSketchMinCorrelation     float64 // Default: 2.0
    GraphSketchEdgeLimit          int     // Default: 200

    // TimeCluster
    TimeClusterSlackSeconds int64  // Default: 1
}
```

### 2.2 Tuning Harness Script

```python
# comp/observer/tuning/harness.py
import optuna
import subprocess
import json
import os

class TuningHarness:
    def __init__(self, parquet: str, scenario: str, detector: str, correlator: str):
        self.parquet = parquet
        self.scenario = scenario  # "memory-leak" or "network-latency"
        self.detector = detector  # "cusum" or "lightesd"
        self.correlator = correlator  # "graphsketch" or "timecluster"

    def objective(self, trial: optuna.Trial) -> float:
        # 1. Sample parameters based on detector/correlator choice
        params = self.sample_params(trial)

        # 2. Run replay with params
        output_json = self.run_replay(params)

        # 3. GPT-5.2 diagnosis
        diagnosis = self.diagnose(output_json)

        # 4. GPT-5.2 score
        score = self.grade(diagnosis)

        return score

    def sample_params(self, trial):
        params = {}

        if self.detector == "cusum":
            params["cusum_baseline_fraction"] = trial.suggest_float("cusum_baseline_fraction", 0.1, 0.5)
            params["cusum_slack_factor"] = trial.suggest_float("cusum_slack_factor", 0.1, 2.0)
            params["cusum_threshold_factor"] = trial.suggest_float("cusum_threshold_factor", 2.0, 8.0)
        elif self.detector == "lightesd":
            params["lightesd_min_window_size"] = trial.suggest_int("lightesd_min_window", 20, 200)
            params["lightesd_alpha"] = trial.suggest_float("lightesd_alpha", 0.001, 0.1, log=True)
            params["lightesd_trend_window_fraction"] = trial.suggest_float("lightesd_trend_frac", 0.05, 0.3)
            params["lightesd_periodicity_significance"] = trial.suggest_float("lightesd_period_sig", 0.001, 0.1, log=True)
            params["lightesd_max_periods"] = trial.suggest_int("lightesd_max_periods", 1, 4)

        if self.correlator == "graphsketch":
            params["graphsketch_cooccurrence_window"] = trial.suggest_int("gs_cooccurrence", 1, 60)
            params["graphsketch_decay_factor"] = trial.suggest_float("gs_decay", 0.5, 0.99)
            params["graphsketch_min_correlation"] = trial.suggest_float("gs_min_corr", 0.5, 10.0)
            params["graphsketch_edge_limit"] = trial.suggest_int("gs_edge_limit", 50, 1000)
        elif self.correlator == "timecluster":
            params["timecluster_slack_seconds"] = trial.suggest_int("tc_slack", 1, 30)

        return params

    def run_replay(self, params) -> str:
        """Run observer with params, return output JSON path."""
        output_path = f"/tmp/observer_{os.getpid()}.json"

        cmd = ["./bin/observer-demo-v2",
               "--parquet", self.parquet,
               "--detector", self.detector,
               "--correlator", self.correlator,
               "--output-json", output_path]

        # Add param flags
        for key, value in params.items():
            cmd.append(f"--{key.replace('_', '-')}={value}")

        subprocess.run(cmd, check=True, capture_output=True)
        return output_path

    def diagnose(self, json_path: str) -> str:
        """Call analyze_with_llm.py with GPT-5.2."""
        result = subprocess.run(
            ["python3", "analyze_with_llm.py", json_path,
             "--model", "gpt-5.2-2025-12-11",
             f"--{self.detector}", f"--{self.correlator}"],
            capture_output=True, text=True, check=True
        )
        return result.stdout

    def grade(self, diagnosis: str) -> float:
        """Call evaluate_diagnosis.py with GPT-5.2."""
        # Write diagnosis to temp file
        with open("/tmp/diagnosis.txt", "w") as f:
            f.write(diagnosis)

        result = subprocess.run(
            ["python3", "evaluate_diagnosis.py", "/tmp/diagnosis.txt",
             "--scenario", self.scenario],
            capture_output=True, text=True, check=True
        )

        # Parse score from output
        for line in result.stdout.split("\n"):
            if "Score:" in line:
                return float(line.split(":")[1].strip())
        return 0.0


def main():
    import argparse
    parser = argparse.ArgumentParser()
    parser.add_argument("--parquet", required=True)
    parser.add_argument("--scenario", required=True, choices=["memory-leak", "network-latency"])
    parser.add_argument("--detector", required=True, choices=["cusum", "lightesd"])
    parser.add_argument("--correlator", required=True, choices=["graphsketch", "timecluster"])
    parser.add_argument("--trials", type=int, default=30)
    args = parser.parse_args()

    harness = TuningHarness(args.parquet, args.scenario, args.detector, args.correlator)

    study = optuna.create_study(direction="maximize")
    study.optimize(harness.objective, n_trials=args.trials)

    print(f"\nBest score: {study.best_value}")
    print(f"Best params: {study.best_params}")

    # Save results
    study.trials_dataframe().to_csv(
        f"tuning_{args.scenario}_{args.detector}_{args.correlator}.csv"
    )

if __name__ == "__main__":
    main()
```

**Usage:**
```bash
python3 comp/observer/tuning/harness.py \
    --parquet memory-leak-export.parquet \
    --scenario memory-leak \
    --detector cusum \
    --correlator timecluster \
    --trials 30
```

---

## Step 3: Run Tuning

### Experiments

**Memory leak** (current best: CUSUM + TimeCluster = 80/100):

| Detector | Correlator | Current Score | Goal |
|----------|------------|---------------|------|
| CUSUM | TimeCluster | 80 | >85 |
| CUSUM | GraphSketch | 61 | >70 |
| LightESD | TimeCluster | 9 | >40 |
| LightESD | GraphSketch | 25 | >40 |

**Network latency** (all fail currently):

| Detector | Correlator | Current Score | Goal |
|----------|------------|---------------|------|
| CUSUM | TimeCluster | 7 | >40 |
| CUSUM | GraphSketch | 5 | >40 |
| LightESD | TimeCluster | 8 | >40 |
| LightESD | GraphSketch | 7 | >40 |

Start with 30 trials each. 8 experiments × 30 trials = 240 total trials.

---

## Step 4: (Later) New Algorithms

After tuning, if results still poor:

- **Hybrid detector**: Combine CUSUM + LightESD
- **F-FADE v2**: Detect unusual edges (not just frequent ones)

---

## Implementation Order

1. [ ] Fix GraphSketch cache → validate no backpressure
2. [ ] Add config flags to demo_main_v2.go
3. [ ] Create tuning harness.py
4. [ ] Run tuning experiments:
   - [ ] Memory leak: CUSUM + TimeCluster (30 trials)
   - [ ] Memory leak: CUSUM + GraphSketch (30 trials)
   - [ ] Memory leak: LightESD + TimeCluster (30 trials)
   - [ ] Memory leak: LightESD + GraphSketch (30 trials)
   - [ ] Network latency: CUSUM + TimeCluster (30 trials)
   - [ ] Network latency: CUSUM + GraphSketch (30 trials)
   - [ ] Network latency: LightESD + TimeCluster (30 trials)
   - [ ] Network latency: LightESD + GraphSketch (30 trials)
5. [ ] Analyze results, pick best configs
6. [ ] (Later) Try hybrid/F-FADE if needed

---

## Parameter Search Spaces

### CUSUM (ts_analysis_cusum.go:29-47)
| Parameter | Default | Range | Notes |
|-----------|---------|-------|-------|
| baseline_fraction | 0.25 | [0.1, 0.5] | Fraction of data for baseline estimation |
| slack_factor | 0.5 | [0.1, 2.0] | Multiplier for stddev → slack k |
| threshold_factor | 4.0 | [2.0, 8.0] | Multiplier for stddev → threshold h |

### LightESD (emitter_lightesd.go:16-64)
| Parameter | Default | Range | Notes |
|-----------|---------|-------|-------|
| min_window_size | 50 | [20, 200] | Min points for analysis |
| alpha | 0.05 | [0.001, 0.1] | Significance level (lower = stricter) |
| trend_window_fraction | 0.15 | [0.05, 0.3] | Fraction for trend smoothing |
| periodicity_significance | 0.01 | [0.001, 0.1] | p-value for seasonality detection |
| max_periods | 2 | [1, 4] | Max seasonal components |

### GraphSketch (anomaly_processor_graphsketch.go:19-56)
| Parameter | Default | Range | Notes |
|-----------|---------|-------|-------|
| cooccurrence_window | 10 | [1, 60] | Seconds to consider co-occurring |
| decay_factor | 0.85 | [0.5, 0.99] | Time decay (higher = slower decay) |
| min_correlation_strength | 2.0 | [0.5, 10.0] | Min edge frequency to cluster |
| edge_limit | 200 | [50, 1000] | Top-K edges to track |

### TimeCluster (anomaly_processor_time_cluster.go:16-38)
| Parameter | Default | Range | Notes |
|-----------|---------|-------|-------|
| slack_seconds | 1 | [1, 30] | Time slack for grouping |
