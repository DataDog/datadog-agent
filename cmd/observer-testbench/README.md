# Observer Test Bench

A standalone tool for iterating on observer anomaly detection and correlation algorithms. Load historical scenarios and visualize analysis results in a web UI, or run headless for batch evaluation.

## Quick Start

```bash
# Build the testbench
go build -o observer-testbench ./cmd/observer-testbench

# Run with default settings
./observer-testbench --scenarios-dir ./comp/observer/scenarios

# Start the UI (in a separate terminal)
cd cmd/observer-testbench/ui
npm install  # first time only
npm run dev
```

Then open http://localhost:5173 in your browser.

## Command Line Flags

### General

| Flag | Default | Description |
|------|---------|-------------|
| `--scenarios-dir` | `./scenarios` | Directory containing scenario subdirectories |
| `--http` | `:8080` | HTTP server address for the API (interactive mode only) |
| `--enable` | _(empty)_ | Comma-separated components to enable (overrides defaults) |
| `--disable` | _(empty)_ | Comma-separated components to disable (overrides defaults) |
| `--cusum-include-count` | `false` | Include `:count` metrics in CUSUM analysis (skipped by default as they're often noisy) |

### Headless Mode

| Flag | Default | Description |
|------|---------|-------------|
| `--headless` | _(empty)_ | Scenario subdirectory name to run in headless mode (no HTTP server) |
| `--output` | _(empty)_ | Path for observer JSON output |
| `--verbose` | `false` | Include full detail in JSON output (titles, member series, individual anomalies) |

## Components

Components are controlled via `--enable` and `--disable` using their names.

### Detectors

| Name | Default | Description |
|------|---------|-------------|
| `cusum` | enabled | CUSUM change-point detector |
| `bocpd` | enabled | Bayesian Online Change Point Detection |

### Correlators

| Name | Default | Description |
|------|---------|-------------|
| `time_cluster` | enabled | Groups anomalies that occur close together in time |
| `lead_lag` | disabled | Finds temporal causality (which sources consistently precede others) |
| `surprise` | disabled | Finds lift-based patterns (sources that co-occur more/less than expected) |

### Processing

| Name | Default | Description |
|------|---------|-------------|
| `dedup` | disabled | Anomaly deduplication before correlation (reduces noise from repeated detections) |

## Examples

```bash
# Run with all defaults
./observer-testbench --scenarios-dir ./comp/observer/scenarios

# Enable lead-lag correlator
./observer-testbench --scenarios-dir ./comp/observer/scenarios \
  --enable lead_lag

# Only CUSUM + TimeCluster (disable everything else)
./observer-testbench --scenarios-dir ./comp/observer/scenarios \
  --disable bocpd,lead_lag,surprise

# Include :count metrics in analysis
./observer-testbench --scenarios-dir ./comp/observer/scenarios \
  --cusum-include-count

# Run on a different port
./observer-testbench --scenarios-dir ./comp/observer/scenarios \
  --http :9090
```

## Headless Mode

Run a scenario without the HTTP server or UI — load data, run the full detector→correlator pipeline, write a JSON results file, and exit.

The `--headless` value is the name of a subdirectory inside `--scenarios-dir`. For example, given:

```
my-scenarios/
  ├── scenario_a/
  │   └── parquet/
  └── scenario_b/
      └── parquet/
```

`--headless scenario_a --scenarios-dir ./my-scenarios` loads `./my-scenarios/scenario_a/`.

```bash
# Basic headless run
./observer-testbench \
  --headless <scenario-name> \
  --output results.json \
  --scenarios-dir ./path/to/scenarios

# Verbose output (includes anomaly detail, member series, titles)
./observer-testbench \
  --headless <scenario-name> \
  --output results-verbose.json \
  --verbose \
  --scenarios-dir ./path/to/scenarios

# With component overrides
./observer-testbench \
  --headless <scenario-name> \
  --output results.json \
  --scenarios-dir ./path/to/scenarios \
  --enable cusum,time_cluster \
  --disable bocpd,lead_lag,surprise
```

### Output Format

**Non-verbose** — anomaly periods with time spans only:
```json
{
  "metadata": {
    "scenario": "...",
    "timeline_start": 1708200000,
    "timeline_end": 1708203600,
    "detectors_enabled": ["cusum"],
    "correlators_enabled": ["time_cluster"],
    "total_anomaly_periods": 5
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
      "member_series": ["parquet|cpu.user:avg|tag1:v1"],
      "anomalies": [
        {
          "timestamp": 1708201234,
          "source": "cpu.user:avg",
          "source_series_id": "parquet|cpu.user:avg|tag1:v1",
          "detector": "cusum"
        }
      ]
    }
  ]
}
```

## Scenario Directory Structure

Each scenario is a subdirectory within `--scenarios-dir`. Parquet files can live in a `parquet/` subdirectory or directly in the scenario root.

```
scenario-name/
  ├── parquet/                # Parquet metric files (preferred layout)
  │   ├── observer-metrics-*.parquet
  │   └── ...
  └── logs/                   # Log files (optional)
      └── ...
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
| GET | `/api/correlations` | Get correlation outputs |
| GET | `/api/correlations/compressed` | Get compressed correlation outputs |
| GET | `/api/correlators/{name}` | Get data for a specific correlator |
| GET | `/api/leadlag` | Get LeadLag edges (if enabled) |
| GET | `/api/surprise` | Get Surprise edges (if enabled) |
| GET | `/api/stats` | Correlator statistics |
| POST | `/api/config` | Update runtime configuration |

## UI Features

- **Scenario Selection**: Load different scenarios from the sidebar
- **Component Toggles**: Enable/disable detectors and correlators
- **Series Tree**: Browse and select time series to visualize
- **Aggregation Types**: Switch between avg, count, sum, min, max views
- **Time Clusters**: View correlated anomaly groups
- **Lead-Lag Edges**: See temporal causality relationships
- **Surprise Patterns**: View unexpected co-occurrence patterns
- **Zoom/Pan**: Drag to zoom, middle-drag to pan on charts
- **Split by Tag**: Split series by tag values for comparison
