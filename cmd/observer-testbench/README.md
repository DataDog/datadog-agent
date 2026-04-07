# Observer Test Bench

A standalone tool for iterating on observer anomaly detection and correlation algorithms. Load historical scenarios and visualize analysis results in a web UI, or run headless for batch evaluation and hyperparameter optimization.

## Quick Start

```bash
# Build + launch backend and UI in one command (recommended)
dda inv -- q.launch-testbench --build

# Just launch (if already built)
dda inv -- q.launch-testbench

# Use a custom scenarios directory
dda inv -- q.launch-testbench --scenarios-dir /path/to/scenarios
```

Then open http://localhost:5173 in your browser.

The `--build` flag rebuilds the binary before launching. Omit it after the first run to skip the build step.

### Manual launch

```bash
# Build only
dda inv -- q.build-testbench

# Run the backend
./bin/observer-testbench --scenarios-dir ./comp/observer/scenarios

# Start the UI (in a separate terminal)
cd cmd/observer-testbench/ui
npm install  # first time only
npm run dev
```

## Command Line Flags

### General

| Flag | Default | Description |
|------|---------|-------------|
| `--scenarios-dir` | `./comp/observer/scenarios` | Directory containing scenario subdirectories |
| `--http` | `:8080` | HTTP server address for the API (interactive mode only) |
| `--enable` | _(empty)_ | Comma-separated components to enable (overrides defaults) |
| `--disable` | _(empty)_ | Comma-separated components to disable (overrides defaults) |
| `--only` | _(empty)_ | Enable ONLY these components (plus extractors); disable everything else. Mutually exclusive with `--enable`/`--disable`. |
| `--config` | _(empty)_ | Path to a JSON params file controlling enabled state and hyperparameters. Takes full precedence over `--enable`/`--disable`/`--only`. See [Params File](#params-file--config). |

### Headless Mode

| Flag | Default | Description |
|------|---------|-------------|
| `--headless` | _(empty)_ | Scenario subdirectory name to run in headless mode (no HTTP server) |
| `--output` | _(empty)_ | Path for observer JSON output |
| `--verbose` | `false` | Include full detail in JSON output (titles, member series, individual anomalies) |
| `--memprofile` | _(empty)_ | Write a heap profile to this file after the run |

## Components

### Detectors

| Name | Default | Description |
|------|---------|-------------|
| `bocpd` | enabled | Bayesian Online Change Point Detection — streaming, per-series changepoint detector |
| `rrcf` | enabled | Robust Random Cut Forest — multivariate anomaly detection over system metrics |
| `cusum` | disabled | CUSUM change-point detector |
| `scanmw` | disabled | Mann-Whitney scan statistic detector |
| `scanwelch` | disabled | Welch t-test scan statistic detector |

### Correlators

| Name | Default | Description |
|------|---------|-------------|
| `time_cluster` | enabled | Groups anomalies that occur close together in time |
| `cross_signal` | disabled | Cross-signal pattern correlator (fixed known patterns) |
| `passthrough` | disabled | Passes every anomaly through as its own correlation (for TP metric scoring) |

### Extractors

Extractors are always enabled and convert raw observations into timeseries:

| Name | Description |
|------|-------------|
| `log_metrics_extractor` | Derives metrics from log patterns and JSON fields |
| `connection_error_extractor` | Detects connection-error patterns in logs |
| `log_pattern_extractor` | Clusters log messages into patterns |

## Examples

```bash
# Run with all defaults (bocpd + rrcf + time_cluster)
dda inv -- q.launch-testbench

# Only BOCPD + TimeCluster
dda inv -- q.launch-testbench --only bocpd,time_cluster

# Enable CUSUM on top of defaults
dda inv -- q.launch-testbench --enable cusum

# Run on a different port
dda inv -- q.launch-testbench --http :9090
```

## Headless Mode

Run a scenario without the HTTP server — load data, run the full detector→correlator pipeline, write a JSON results file, and exit.

```bash
# Via invoke (builds automatically with --build)
dda inv -- q.launch-testbench --headless-scenario <scenario-name>
dda inv -- q.launch-testbench --headless-scenario <scenario-name> --headless-output /tmp/out.json
dda inv -- q.launch-testbench --headless-scenario <scenario-name> --profile  # write heap profile

# Direct binary
./bin/observer-testbench \
  --headless <scenario-name> \
  --output results.json \
  --scenarios-dir ./comp/observer/scenarios

# Verbose output (includes anomaly detail, member series, titles)
./bin/observer-testbench \
  --headless <scenario-name> \
  --output results-verbose.json \
  --verbose \
  --scenarios-dir ./comp/observer/scenarios

# Only BOCPD + TimeCluster in headless mode
./bin/observer-testbench \
  --headless <scenario-name> \
  --output results.json \
  --scenarios-dir ./comp/observer/scenarios \
  --only bocpd,time_cluster
```

### Output Format

**Non-verbose** — anomaly periods with time spans only:
```json
{
  "metadata": {
    "scenario": "my_scenario",
    "timeline_start": 1708200000,
    "timeline_end": 1708203600,
    "detectors_enabled": ["bocpd"],
    "correlators_enabled": ["time_cluster"],
    "total_anomaly_periods": 2
  },
  "anomaly_periods": [
    {
      "pattern": "cluster_1708201234",
      "period_start": 1708201234,
      "period_end": 1708201500
    }
  ]
}
```

**Verbose** (`--verbose`) — adds title, member series, and nested anomalies:
```json
{
  "anomaly_periods": [
    {
      "pattern": "cluster_1708201234",
      "period_start": 1708201234,
      "period_end": 1708201500,
      "title": "Correlated anomalies at 1708201234",
      "member_series": ["parquet|cpu.user:avg|host:web-1"],
      "anomalies": [
        {
          "timestamp": 1708201234,
          "source": "cpu.user:avg",
          "source_series_id": "42:avg",
          "detector": "bocpd_detector"
        }
      ]
    }
  ]
}
```

## Params File (`--config`)

The `--config` flag accepts a JSON file that controls both enabled/disabled state and hyperparameters for any component. This is the recommended interface for **Bayesian hyperparameter optimization** — the optimizer writes a params file, runs the testbench in headless mode, and scores the output with `observer-scorer`.

When `--config` is provided it takes full precedence over `--enable`/`--disable`/`--only`. Components not mentioned in the file use their catalog defaults.

### Schema

```json
{
  "components": {
    "<component-name>": {
      "enabled": true,
      "<param>": <value>,
      ...
    }
  }
}
```

- `"enabled"` is optional — omit to use the catalog default.
- Hyperparameter fields are optional — omitted fields keep their default values.

### Example

```json
{
  "components": {
    "bocpd": {
      "enabled": true,
      "hazard": 0.08,
      "cp_threshold": 0.55,
      "cp_mass_threshold": 0.65,
      "recovery_points": 8
    },
    "rrcf": {
      "enabled": true,
      "threshold_sigma": 2.5,
      "tree_size": 128
    },
    "cusum": { "enabled": false },
    "time_cluster": {
      "enabled": true,
      "proximity_seconds": 15,
      "window_seconds": 180,
      "min_cluster_size": 2
    }
  }
}
```

### Available Hyperparameters

#### `bocpd`

| Param | Default | Description |
|-------|---------|-------------|
| `warmup_points` | 120 | Points used to build the initial baseline |
| `hazard` | 0.05 | Prior P(changepoint) per time step — primary sensitivity knob |
| `cp_threshold` | 0.6 | Posterior P(changepoint) threshold to fire — primary FP/FN knob |
| `short_run_length` | 5 | Secondary trigger: run-length horizon k |
| `cp_mass_threshold` | 0.7 | Secondary trigger: P(r_t ≤ k) threshold |
| `max_run_length` | 200 | Compute cap for tracked hypotheses |
| `prior_variance_scale` | 10.0 | Diffuseness of prior over the mean |
| `min_variance` | 1.0 | Variance floor (numerical stability) |
| `recovery_points` | 10 | Consecutive quiet points needed to exit alert state |

#### `cusum`

| Param | Default | Description |
|-------|---------|-------------|
| `min_points` | 5 | Minimum data points required |
| `baseline_fraction` | 0.25 | Fraction of data used for baseline estimation |
| `slack_factor` | 0.5 | k = slack_factor × stddev (slack parameter) |
| `threshold_factor` | 4.0 | h = threshold_factor × stddev (detection threshold) |

#### `rrcf`

| Param | Default | Description |
|-------|---------|-------------|
| `num_trees` | 100 | Number of trees in the forest |
| `tree_size` | 256 | Sliding window size per tree |
| `shingle_size` | 4 | Temporal context window (consecutive samples per point) |
| `threshold_sigma` | 3.0 | σ above score mean to flag an anomaly |

#### `time_cluster`

| Param | Default | Description |
|-------|---------|-------------|
| `proximity_seconds` | 10 | Max time gap between anomalies to consider them in the same cluster |
| `window_seconds` | 120 | How long to keep anomalies before eviction |
| `min_cluster_size` | 0 | Minimum cluster size to report (0 = report all) |

#### `cross_signal`

| Param | Default | Description |
|-------|---------|-------------|
| `window_seconds` | 30 | Time window for clustering anomalies |

## Scenario Directory Structure

Each scenario is a subdirectory within `--scenarios-dir`. An optional `episode.json` provides ground truth for scoring.

```
scenario-name/
  ├── parquet/
  │   ├── observer-metrics-*.parquet    # Metric time series
  │   ├── observer-logs-*.parquet       # Log entries
  │   └── ...
  └── episode.json                      # Ground truth (disruption onset, baseline window)
```

### `episode.json` format

```json
{
  "baseline": { "start": "2024-02-17T11:00:00Z", "end": "2024-02-17T11:30:00Z" },
  "disruption": { "start": "2024-02-17T12:00:00Z" }
}
```

## API Endpoints

These endpoints are available in interactive mode (not headless).

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/status` | Server status and loaded scenario info |
| GET | `/api/scenarios` | List available scenarios |
| POST | `/api/scenarios/{name}/load` | Load a scenario |
| GET | `/api/components` | List registered components |
| POST | `/api/components/{name}/toggle` | Toggle a component on/off |
| GET | `/api/series` | List all time series |
| GET | `/api/series/{ns}/{name}` | Get series data by namespace and name |
| GET | `/api/series/id/{id}` | Get series data by ID |
| GET | `/api/anomalies` | Get all detected anomalies |
| GET | `/api/logs` | Get loaded log entries |
| GET | `/api/log-anomalies` | Get log anomalies |
| GET | `/api/log-patterns` | Get log pattern clusters |
| GET | `/api/correlations` | Get correlation outputs |
| GET | `/api/correlations/compressed` | Get compressed correlation outputs |
| GET | `/api/reports` | Datadog-style report events |
| GET | `/api/score` | Gaussian F1 score against episode.json ground truth |
| GET | `/api/stats` | Correlator statistics |
| GET | `/api/benchmark` | Component benchmark data |

## UI Features

- **Scenario Selection**: Load different scenarios from the sidebar
- **Component Toggles**: Enable/disable detectors and correlators live
- **Series Tree**: Browse and select time series to visualize
- **Aggregation Types**: Switch between avg, count, sum, min, max views
- **Time Clusters**: View correlated anomaly groups
- **Zoom/Pan**: Drag to zoom, middle-drag to pan on charts
- **Split by Tag**: Split series by tag values for comparison
- **Log Patterns**: Browse detected log pattern clusters
- **Reports**: Datadog-style incident events on a zoomable timeline
- **Score**: Live Gaussian F1 score against episode.json ground truth
