# Observer Test Bench

A standalone tool for iterating on observer anomaly detection and correlation algorithms. Load historical scenarios and visualize analysis results in a web UI.

## Quick Start

```bash
# Build the testbench
go build -o observer-testbench ./cmd/observer-testbench

# Run with default settings (all correlators enabled)
./observer-testbench --scenarios-dir ./comp/observer/anomaly_datasets_converted

# Start the UI (in a separate terminal)
cd cmd/observer-testbench/ui
npm install  # first time only
npm run dev
```

Then open http://localhost:5173 in your browser.

## Command Line Flags

### Required

| Flag | Default | Description |
|------|---------|-------------|
| `--scenarios-dir` | `./scenarios` | Directory containing scenario subdirectories (parquet files, logs, events) |

### Server

| Flag | Default | Description |
|------|---------|-------------|
| `--http` | `:8080` | HTTP server address for the API |

### Detectors

| Flag | Default | Description |
|------|---------|-------------|
| `--cusum` | `true` | Enable CUSUM change-point detector |
| `--zscore` | `true` | Enable Robust Z-Score detector |
| `--cusum-include-count` | `false` | Include `:count` metrics in CUSUM analysis (default: skip them as they're often noisy) |

### Correlators

| Flag | Default | Description |
|------|---------|-------------|
| `--time-cluster` | `true` | Enable TimeCluster correlator - groups anomalies that occur close together in time |
| `--lead-lag` | `true` | Enable LeadLag correlator - finds temporal causality (which sources consistently precede others) |
| `--surprise` | `true` | Enable Surprise correlator - finds lift-based patterns (sources that co-occur more/less than expected) |
| `--graph-sketch` | `true` | Enable GraphSketch correlator - learns co-occurrence patterns with decay |

### Processing

| Flag | Default | Description |
|------|---------|-------------|
| `--dedup` | `false` | Enable anomaly deduplication before correlation (reduces noise from repeated detections) |

## Examples

```bash
# Run with all defaults (recommended for most cases)
./observer-testbench --scenarios-dir ./comp/observer/anomaly_datasets_converted

# Disable noisy correlators for cleaner output
./observer-testbench --scenarios-dir ./comp/observer/anomaly_datasets_converted \
  --surprise=false --graph-sketch=false

# Include :count metrics in analysis
./observer-testbench --scenarios-dir ./comp/observer/anomaly_datasets_converted \
  --cusum-include-count

# Enable deduplication for cleaner correlation results
./observer-testbench --scenarios-dir ./comp/observer/anomaly_datasets_converted \
  --dedup

# Minimal setup - just CUSUM detector with TimeCluster
./observer-testbench --scenarios-dir ./comp/observer/anomaly_datasets_converted \
  --zscore=false --lead-lag=false --surprise=false --graph-sketch=false

# Run on a different port
./observer-testbench --scenarios-dir ./comp/observer/anomaly_datasets_converted \
  --http :9090
```

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/status` | Server status and loaded scenario info |
| GET | `/api/scenarios` | List available scenarios |
| POST | `/api/scenarios/{name}/load` | Load a scenario |
| GET | `/api/components` | List registered components (detectors, correlators) |
| GET | `/api/series` | List all time series |
| GET | `/api/series/{ns}/{name}` | Get data for a specific series |
| GET | `/api/anomalies` | Get all detected anomalies |
| GET | `/api/correlations` | Get TimeCluster correlation outputs |
| GET | `/api/leadlag` | Get LeadLag edges (if enabled) |
| GET | `/api/surprise` | Get Surprise edges (if enabled) |
| GET | `/api/graphsketch` | Get GraphSketch edges (if enabled) |
| GET | `/api/stats` | Get correlator statistics |

## Scenario Directory Structure

Each scenario should be a subdirectory containing:

```
scenario-name/
  ├── metrics.parquet    # Time series data (required)
  ├── logs.json          # Log events (optional)
  └── events.json        # Other events (optional)
```

## UI Features

- **Scenario Selection**: Load different scenarios from the sidebar
- **Analyzer Toggles**: Enable/disable specific detectors in the view
- **Series Tree**: Browse and select time series to visualize
- **Aggregation Types**: Switch between avg, count, sum, min, max views
- **Time Clusters**: View correlated anomaly groups
- **Lead-Lag Edges**: See temporal causality relationships
- **Surprise Patterns**: View unexpected co-occurrence patterns
- **GraphSketch Edges**: See learned co-occurrence relationships
- **Zoom/Pan**: Drag to zoom, middle-drag to pan on charts
- **Split by Tag**: Split series by tag values for comparison
