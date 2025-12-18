# Log Drift Detection Component

An unsupervised anomaly detection system for log streams using Dynamic Mode Decomposition (DMD) and semantic embeddings.

## Overview

This component implements the drift detection pipeline described in `drift_detection_doc/`. It processes logs in real-time to identify behavioral anomalies by:

1. **Compressing** logs into templates using Shannon entropy
2. **Embedding** templates into 768-dimensional vectors via HTTP API
3. **Analyzing** temporal dynamics with Hankel DMD
4. **Alerting** on statistical deviations (2σ/3σ thresholds)

### Per-Source Mode

The drift detector operates in **per-source mode**, creating a separate pipeline for each unique log source. This provides:
- **Source isolation**: Each source has its own DMD model
- **Better accuracy**: Different log patterns per service are properly handled
- **Source-specific alerts**: Know exactly which service is experiencing drift
- **Automatic cleanup**: Idle sources are removed to conserve resources

See [PER_SOURCE.md](PER_SOURCE.md) for detailed documentation on per-source drift detection.

## Architecture

```
comp/logs/driftdetector/
├── def/
│   └── component.go          # Component interface
├── fx/
│   └── fx.go                 # FX module definition
├── impl/
│   ├── driftdetector.go      # Main component implementation
│   ├── common/
│   │   └── types.go          # Shared types and config
│   ├── window/
│   │   └── manager.go        # Sliding window aggregation (120s/60s)
│   ├── template/
│   │   └── extractor.go      # Shannon entropy-based template extraction
│   ├── embedding/
│   │   └── client.go         # HTTP client with batching (100 templates, 5s timeout)
│   ├── dmd/
│   │   └── analyzer.go       # Hankel DMD + anomaly detection
│   ├── alert/
│   │   └── manager.go        # Alerting with Prometheus metrics
│   └── pipeline/
│       └── pipeline.go       # Pipeline orchestration
└── README.md
```

## Configuration

Add to `datadog.yaml`:

```yaml
logs_config:
  drift_detection:
    enabled: false                                    # Enable/disable drift detection
    embedding_url: "http://localhost:11434/api/embed"  # Ollama embedding service URL
    embedding_model: "embeddinggemma"                 # Ollama embedding model name
    window_size: 120s                                 # Sliding window size
    window_step: 60s                                  # Window step (50% overlap)
    entropy_threshold: 2.5                            # Shannon entropy threshold for variable detection
    warning_threshold: 2.0                            # Standard deviations for WARNING alert
    critical_threshold: 3.0                           # Standard deviations for CRITICAL alert
    dmd_time_delay: 5                                 # Hankel time-delay depth
    dmd_rank: 50                                      # SVD rank for dimensionality reduction
    cleanup_interval: 5m                              # How often to check for idle sources
    max_idle_time: 30m                                # Remove sources idle for this duration
```

## How It Works

### 1. Template Extraction (Shannon Entropy)

Reduces log variability by detecting constant, enum, and variable fields:

```
Input:
  2025-12-10 14:23:45 UTC | CORE | INFO | Server started on port 8080
  2025-12-10 14:23:46 UTC | CORE | INFO | Server started on port 8081

Output:
  2025-12-10 <*> UTC | CORE | INFO | Server started on port <*>
```

- **Low entropy** (≤2.5): Constant field (keep original)
- **High entropy + low cardinality** (<10 unique): Enum field (keep original)
- **High entropy + high cardinality**: Variable field (replace with `<*>`)

### 2. Embedding Generation

Templates are batched (max 100, timeout 5s) and sent to embedding service using Ollama API:

```json
POST /api/embed
{
  "model": "embeddinggemma",
  "input": ["template1", "template2"]
}
```

Returns 768-dimensional vectors for semantic similarity in Ollama format:
```json
{
  "embeddings": [
    [0.1, 0.2, ...],
    [0.3, 0.4, ...]
  ],
  "model": "embeddinggemma"
}
```

### 3. Hankel DMD Analysis

**Dynamic Mode Decomposition (DMD)** learns normal system dynamics:

1. **Time-delay embedding**: Augments state space (768 → 3840 dims) to capture 5-step temporal dependencies
2. **SVD decomposition**: Extracts spatial modes and temporal eigenvalues
3. **Reconstruction**: Predicts expected behavior from learned patterns
4. **Error calculation**: L2 norm between actual and predicted states
5. **Normalization**: Convert to standard deviations (μ, σ)

**Anomaly Detection**:
- `normalized_error > 2σ` → **WARNING**
- `normalized_error > 3σ` → **CRITICAL**

### 4. Alerting

Logs structured JSON alerts and updates Prometheus metrics:

```json
{
  "timestamp": "2025-12-10T14:23:45Z",
  "level": "WARN",
  "message": "Anomaly detected in log stream",
  "window_id": 42,
  "reconstruction_error": 2.34,
  "normalized_error": 2.1,
  "severity": "warning",
  "templates": ["template1", "template2"]
}
```

## Prometheus Metrics

- `logdrift_anomalies_detected_total{severity="warning|critical"}` - Total anomalies detected
- `logdrift_dmd_reconstruction_error` - Current reconstruction error
- `logdrift_dmd_normalized_error` - Normalized error (standard deviations)

## Performance Characteristics

**Per source** (with automatic cleanup):
- **Memory**: ~11 MB per active source
- **Goroutines**: 10 per active source
- **CPU**: ~2-5% per source on modern CPU

**Scaling examples**:
- 10 sources: ~110 MB, 100 goroutines
- 50 sources: ~550 MB, 500 goroutines
- 100 sources: ~1.1 GB, 1,000 goroutines

**Automatic resource management**:
- Idle sources (no logs for 30min) are automatically removed
- Sources are recreated on-demand when they become active again
- Cleanup runs every 5 minutes (configurable)

## Dependencies

- `gonum.org/v1/gonum` - Matrix operations for DMD
- `github.com/prometheus/client_golang` - Metrics
- **Embedding Service**: Ollama with embedding model (e.g., `embeddinggemma`)

## Usage

### Automatic Integration

The drift detector is **automatically integrated into the logs processor pipeline**. Every log that passes through the agent is automatically sent to the drift detector when it's enabled. No additional code is required.

The integration flow is:
1. Logs ingested by the agent → Logs Processor
2. Logs Processor → Drift Detector (if enabled)
3. Drift Detector → Template Extraction → Embedding → DMD → Alerting

### Manual Usage (Optional)

If you need to manually send logs to the drift detector from other components:

```go
// Component is injected via FX
type YourComponent struct {
    driftDetector driftdetector.Component
}

// Manually process logs
driftDetector.ProcessLog(timestamp, content)
```

## Implementation Notes

- **Disabled by default**: Set `logs_config.drift_detection.enabled: true` to enable
- **Requires Ollama**: Must run Ollama with an embedding model at configured URL
  - Install Ollama: `curl -fsSL https://ollama.com/install.sh | sh`
  - Pull embedding model: `ollama pull embeddinggemma`
  - Test with: `curl -X POST http://localhost:11434/api/embed -H "Content-Type: application/json" -d '{"model": "embeddinggemma", "input": ["test"]}'`
- **Rolling window**: Maintains 2-hour history (120 windows at 60s step)
- **Recomputation**: DMD recomputed every 10 windows for efficiency
- **Back-pressure**: Drops logs if input channel is full (10,000 capacity)
- **Batching**: Templates are batched (up to 100 per request) for efficiency

## References

- **Documentation**: See `drift_detection_doc/` for detailed algorithms and architecture
- **Research**: Kutz, J. N., et al. (2016). "Dynamic Mode Decomposition: Data-Driven Modeling of Complex Systems"
