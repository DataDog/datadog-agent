# Log Drift Detection Pipeline

An unsupervised anomaly detection system for log streams using Dynamic Mode Decomposition (DMD) and semantic embeddings.

## Overview

This system detects behavioral anomalies in log streams by:
1. **Compressing** logs into templates using Shannon entropy
2. **Embedding** templates into 768-dimensional vectors
3. **Analyzing** temporal dynamics with Hankel DMD
4. **Alerting** on statistical deviations from learned patterns

**Key Features**:
- Real-time processing (1000 logs/second)
- Unsupervised learning (no labeled data required)
- Captures multi-step temporal dependencies
- Resource efficient (<100 MB memory, <50% CPU)
- Production-ready with Prometheus metrics

## Architecture

```
Logs → Windowing → Template Extraction → Embedding → Hankel DMD → Anomaly Detection
       (120s/60s)  (Shannon Entropy)     (HTTP API)   (2h window)  (2σ/3σ threshold)
```

The pipeline operates with 10 concurrent goroutines processing data through buffered channels:
- **4 workers** for parallel template extraction
- **1 manager** per stage (windowing, batching, DMD, detection, alerting)

**See [ARCHITECTURE.md](ARCHITECTURE.md) for detailed pipeline design and data flow diagrams.**

## How It Works

### 1. Template Extraction

Reduces log variability by replacing variable fields with `<*>` placeholders:

```
Input:
  2025-12-10 14:23:45 UTC | CORE | INFO | Server started on port 8080
  2025-12-10 14:23:46 UTC | CORE | INFO | Server started on port 8081

Output:
  2025-12-10 <*> UTC | CORE | INFO | Server started on port <*>
```

**Algorithm**: Position-wise Shannon entropy analysis
- Low entropy (≤2.5) → constant field
- High entropy + low cardinality (<10) → enum field
- High entropy + high cardinality → variable field (replace with `<*>`)

**Result**: 1.3x-14.31x compression (average 1.30x)

### 2. Semantic Embedding

Converts templates to 768-dimensional vectors via HTTP API:

```json
POST /embed
{
  "texts": ["template1", "template2"],
  "model": "embeddinggemma"
}
```

Batching strategy accumulates up to 100 templates or flushes after 5s timeout.

### 3. Hankel DMD Anomaly Detection

**Dynamic Mode Decomposition (DMD)** learns normal system dynamics from embedding time series:

```
X = [v₁ v₂ v₃ ... vₙ]  where vᵢ ∈ ℝ⁷⁶⁸
```

**Hankel extension** captures temporal dependencies using time-delay embedding (d=5):

```
H = [v₁ v₂ v₃ ...]    ← t
    [v₂ v₃ v₄ ...]    ← t+1
    [v₃ v₄ v₅ ...]    ← t+2
    [v₄ v₅ v₆ ...]    ← t+3
    [v₅ v₆ v₇ ...]    ← t+4
```

DMD decomposes this into spatial modes (patterns) and temporal eigenvalues (dynamics), then reconstructs expected behavior:

```
X_reconstructed = Φ × Λᵏ × b
```

**Anomaly detection**: When reconstruction error exceeds statistical thresholds:
- `error > 2σ` → **WARNING**
- `error > 3σ` → **CRITICAL**

**Rolling window**: 2-hour retention (120 windows) in FIFO circular buffer, recomputed every 10 windows for efficiency.

**See [ALGORITHMS.md](ALGORITHMS.md) for mathematical details and complexity analysis.**

## API Reference

The embedding service accepts HTTP POST requests:

**Endpoint**: `POST /embed`

**Request**:
```json
{
  "texts": ["template1", "template2"],
  "model": "embeddinggemma",
  "batch_size": 100,
  "normalize": true
}
```

**Response**:
```json
{
  "embeddings": [[0.123, -0.456, ...], ...],
  "dimension": 768,
  "processing_time_ms": 1234,
  "status": "success"
}
```

**See [API.md](API.md) for complete specification, error codes, and Go client example.**

## Configuration

Key parameters (defined in `pkg/pipeline/config.go`):

```go
WindowConfig{
    Size:    120 * time.Second,  // Window size
    Step:    60 * time.Second,   // Overlap 50%
}

TemplateConfig{
    EntropyThreshold:         2.5,  // Variable detection
    EnumCardinalityThreshold: 10,   // Enum vs variable
    MaxCharacters:            8000, // Embedding limit
}

EmbeddingConfig{
    ServerURL:  "http://localhost:8080",
    Model:      "embeddinggemma",
    BatchSize:  100,
    MaxRetries: 3,
}

DMDConfig{
    TimeDelay:       5,              // Hankel depth
    WindowRetention: 2 * time.Hour,  // 120 windows
    RecomputeEvery:  10,             // Efficiency
}

AlertConfig{
    WarningThreshold:  2.0,  // 2σ
    CriticalThreshold: 3.0,  // 3σ
}
```

## Observability

**Prometheus Metrics**:
```
logdrift_anomalies_detected_total{severity="warning|critical"}
logdrift_dmd_reconstruction_error
logdrift_template_compression_ratio
logdrift_embedding_latency_seconds
logdrift_pipeline_e2e_latency_seconds
```

**Structured Logs** (JSON):
```json
{
  "timestamp": "2025-12-10T14:23:45Z",
  "level": "WARN",
  "message": "Anomaly detected",
  "window_id": 42,
  "reconstruction_error": 2.34,
  "severity": "warning",
  "templates": ["template1", "template2"]
}
```

## Performance

**Targets**:
- Throughput: 1000 logs/second
- Latency (P99): <500ms end-to-end
- Memory: <100 MB steady-state
- CPU: <50% on 2 cores

**Benchmarks** (from Python prototype):
- Template extraction: 2918 logs → 261 windows → 19.3 avg logs/window
- Compression: 1.0x-14.31x range (median 1.0x, average 1.30x)
- DMD computation: ~50ms for 120 windows (3840×120 matrix)

## Integration

The pipeline integrates with existing Agent process:

```go
import "github.com/yourorg/logdrift/pkg/pipeline"

// Initialize pipeline
config := pipeline.NewDefaultConfig()
config.Embedding.ServerURL = "http://localhost:8080"

p, err := pipeline.New(config)
if err != nil {
    log.Fatal(err)
}

// Start background processing
ctx := context.Background()
go p.Start(ctx)

// Process logs from Agent
for log := range agentLogs {
    p.ProcessLog(pipeline.LogEntry{
        Timestamp: log.Timestamp,
        Content:   log.Content,
    })
}
```

## Dependencies

```go
require (
    github.com/prometheus/client_golang v1.17.0  // Metrics
    gonum.org/v1/gonum v0.14.0                   // Matrix operations
    github.com/sony/gobreaker v0.5.0             // Circuit breaker
)
```

## Project Structure

```
sliding_window_embedding/
├── cmd/logdrift/main.go          # Entry point
├── pkg/
│   ├── pipeline/                 # Orchestration
│   ├── window/                   # Sliding window manager
│   ├── template/                 # Entropy-based extraction
│   ├── embedding/                # HTTP client + batcher
│   ├── dmd/                      # Hankel DMD + detector
│   └── common/                   # Shared types
└── docs/
    ├── README.md                 # This file
    ├── ARCHITECTURE.md           # Pipeline design
    ├── ALGORITHMS.md             # Math details
    └── API.md                    # Embedding API
```

## References

**Research**:
- Kutz, J. N., et al. (2016). "Dynamic Mode Decomposition: Data-Driven Modeling of Complex Systems"
- Shannon, C. E. (1948). "A Mathematical Theory of Communication"

**Implementation**:
- Python prototype: `log_drift_detection_with_template.ipynb`
- Template extraction reference: `../lemur_experiment/lib/template_extraction.py`

## License

[Your License Here]
