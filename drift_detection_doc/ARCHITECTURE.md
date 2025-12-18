# Log Drift Detection Pipeline Architecture

## System Overview

The log drift detection pipeline processes logs in real-time to identify behavioral anomalies using Dynamic Mode Decomposition (DMD). The system operates as a component within the Agent process.

## Pipeline Stages

### 1. Log Ingestion
- **Input**: Logs from Agent process via `ProcessLog()` interface
- **Format**: Timestamp + Content string
- **Buffering**: 10,000-capacity channel for burst handling

### 2. Sliding Window Aggregation
- **Window Size**: 120 seconds
- **Step Size**: 60 seconds (50% overlap)
- **Output**: Completed windows sent to template extraction

### 3. Template Extraction (Shannon Entropy Based)

**Algorithm**: Entropy-based log template mining

**Steps**:
1. **Tokenization**: Split logs by whitespace → tokens
2. **Bucketing**: Group logs by token count (structural alignment)
3. **Position-wise Analysis**: For each token position:
   - Calculate Shannon entropy: `H(X) = -Σ p(x) log₂(p(x))`
   - Detect enums: high entropy (>2.5) but low cardinality (<10)
4. **Template Generation**:
   - Low entropy (≤2.5) → Keep constant token
   - High entropy + low cardinality → Keep enum value
   - High entropy + high cardinality → Replace with `<*>`
5. **Deduplication**: Keep first occurrence of each unique template

**Parameters**:
- `ENTROPY_THRESHOLD = 2.5`
- `ENUM_CARDINALITY_THRESHOLD = 10`
- `MAX_CHARS = 8000` (for embedding input)

**Performance**:
- Complexity: O(n×m×log(m)) where n=logs, m=avg tokens
- Observed compression: 1.3x to 14.31x (median: 1.0x, average: 1.30x)

**Example**:
```
Input logs:
  2025-12-10 14:23:45 UTC | CORE | INFO | Server started on port 8080
  2025-12-10 14:23:46 UTC | CORE | INFO | Server started on port 8081
  2025-12-10 14:23:47 UTC | CORE | WARN | Connection timeout after 30s

Output templates:
  2025-12-10 <*> UTC | CORE | INFO | Server started on port <*>
  2025-12-10 <*> UTC | CORE | WARN | Connection timeout after <*>

Compression: 3 logs → 2 templates (1.5x)
```

### 4. Embedding Generation

**Model**: embeddinggemma (768 dimensions)
**API**: Custom HTTP server (POST /embed)

**Batching Strategy**:
- Accumulate up to 100 templates
- Flush on timeout (5s max delay) or batch full
- Parallel requests via connection pool

**Request Format**:
```json
{
  "texts": ["template1", "template2", ...],
  "model": "embeddinggemma",
  "batch_size": 100,
  "normalize": true
}
```

**Response Format**:
```json
{
  "embeddings": [[0.123, -0.456, ...], ...],
  "dimension": 768,
  "processing_time_ms": 1234
}
```

**Error Handling**:
- Exponential backoff retry (3 attempts)
- Circuit breaker pattern
- Connection pooling (10 connections)

### 5. Online Hankel DMD (Anomaly Detection)

**Algorithm**: Dynamic Mode Decomposition with time-delay embedding

**Hankel DMD Overview**:
DMD learns the normal dynamics of a system from a sequence of observations (embeddings). It decomposes the temporal evolution into modes (patterns) and eigenvalues (growth/decay rates). When actual behavior deviates from learned patterns, reconstruction error spikes → anomaly detected.

**Time-Delay Embedding (Hankel)**:
- Parameter: `d=5` (captures 5-step dependencies)
- Augments state space: 768 dims → 3,840 dims (768×5)
- Enables detection of multi-step temporal patterns

**Rolling Window Strategy**:
- **Retention**: 2 hours = 120 windows (at 60s step)
- **Data Structure**: FIFO circular buffer (120 × 768)
- **Recomputation**: Every 10 new windows (avoid per-window overhead)
- **Memory**: ~900 KB for queue (120×768×float64)

**Anomaly Detection Logic**:

1. **Matrix Construction**: Stack embeddings as columns
   ```
   X = [v₁ v₂ v₃ ... vₙ]  where vᵢ ∈ ℝ⁷⁶⁸
   ```

2. **Hankel Transformation**: Time-delay embedding
   ```
   H = [v₁ v₂ v₃ ...]    ← t
       [v₂ v₃ v₄ ...]    ← t+1
       [v₃ v₄ v₅ ...]    ← t+2
       [v₄ v₅ v₆ ...]    ← t+3
       [v₅ v₆ v₇ ...]    ← t+4

   Shape: (3840, n-4)
   ```

3. **DMD Decomposition**: SVD + Eigenvalue decomposition
   - Compute modes Φ and eigenvalues λ
   - Modes capture spatial patterns
   - Eigenvalues capture temporal dynamics
     - |λ| < 1: Decaying (stable)
     - |λ| = 1: Oscillating (stable)
     - |λ| > 1: Growing (unstable, WARNING)

4. **Reconstruction**: Use learned modes to predict
   ```
   X_reconstructed = Φ × Dynamics
   ```

5. **Error Calculation**: Per-window L2 norm
   ```
   error[i] = ||X[:,i] - X_reconstructed[:,i]||₂
   ```

6. **Normalization**: Convert to standard deviations
   ```
   normalized_error = (error - μ) / σ
   ```

7. **Threshold Detection**:
   - `normalized_error > 2.0σ`: **WARNING**
   - `normalized_error > 3.0σ`: **CRITICAL**

**Why It Works**:
- DMD learns "normal" system dynamics from historical data
- High reconstruction error = system behaving differently than learned patterns
- Unsupervised: no labeled training data required
- Captures temporal dependencies (5-step memory via Hankel)

**Performance Characteristics**:
- Computation: ~50ms for 120 windows on modern CPU
- Memory: ~1 MB for queue + ~2 MB for matrices
- Incremental updates: Only recompute every 10 windows

### 6. Alerting & Observability

**Prometheus Metrics**:
- `logdrift_anomalies_detected_total{severity="warning|critical"}`
- `logdrift_dmd_reconstruction_error`
- `logdrift_template_compression_ratio`
- `logdrift_embedding_latency_seconds`
- `logdrift_pipeline_e2e_latency_seconds`

**Log Output** (JSON structured):
```json
{
  "timestamp": "2025-12-10T14:23:45Z",
  "level": "WARN",
  "message": "Anomaly detected",
  "window_id": 42,
  "reconstruction_error": 2.34,
  "severity": "warning",
  "templates": ["template1", "template2", ...]
}
```

## Data Flow Diagram

```
Agent Process
     │
     │ ProcessLog(entry)
     ▼
┌─────────────────────────────────────────────┐
│           Log Ingestion Channel              │
│         (buffered, size: 10000)              │
└─────────────────┬───────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────┐
│          Window Manager (Goroutine)          │
│  • Sliding windows: 120s size, 60s step     │
│  • Accumulates logs per window               │
│  • Triggers on time boundaries               │
└─────────────────┬───────────────────────────┘
                  │ Window{logs: []string}
                  ▼
┌─────────────────────────────────────────────┐
│    Template Extraction Pool (4 workers)     │
│  • Tokenization                              │
│  • Shannon entropy calculation               │
│  • Enum detection                            │
│  • Template generation                       │
└─────────────────┬───────────────────────────┘
                  │ templates: []string
                  ▼
┌─────────────────────────────────────────────┐
│         Embedding Batcher (Goroutine)        │
│  • Accumulates to batch_size (100)          │
│  • Timeout flush (5s)                        │
└─────────────────┬───────────────────────────┘
                  │ batch: []string
                  ▼
┌─────────────────────────────────────────────┐
│     Embedding HTTP Client (Pool: 10)        │
│  POST /embed                                 │
│  • Retry with backoff (3x)                  │
│  • Circuit breaker                           │
└─────────────────┬───────────────────────────┘
                  │ embeddings: []Vector (768-dim)
                  ▼
┌─────────────────────────────────────────────┐
│    DMD Rolling Queue (FIFO, 120 windows)    │
│  • 2-hour retention                          │
│  • Triggers recompute every 10 windows       │
└─────────────────┬───────────────────────────┘
                  │ matrix: (768, 120)
                  ▼
┌─────────────────────────────────────────────┐
│      Hankel DMD Analyzer (Goroutine)        │
│  • Time-delay embedding (d=5)               │
│  • SVD decomposition                         │
│  • Reconstruction error                      │
└─────────────────┬───────────────────────────┘
                  │ errors: []float64
                  ▼
┌─────────────────────────────────────────────┐
│      Anomaly Detector (Goroutine)           │
│  • Normalization (μ, σ)                     │
│  • Threshold: 2σ, 3σ                        │
└─────────────────┬───────────────────────────┘
                  │ alerts: []Alert
                  ▼
┌─────────────────────────────────────────────┐
│       Alert Manager (Goroutine)             │
│  • JSON logging                              │
│  • Prometheus metrics update                 │
└─────────────────────────────────────────────┘
```

## Concurrency Model

**Goroutines** (10 total):
1. **Window Manager**: Accumulates logs, triggers window completion
2-5. **Template Workers** (×4): Parallel template extraction
6. **Embedding Batcher**: Accumulates and flushes batches
7. **DMD Queue Manager**: Maintains rolling window
8. **DMD Analyzer**: Computes Hankel DMD
9. **Anomaly Detector**: Threshold-based detection
10. **Alert Manager**: Logging and metrics

**Channel Strategy**:
- Buffered channels between stages (10-10000 capacity)
- Fan-out for parallel template workers
- Back-pressure handling (blocking on full channels)

**Synchronization**:
- Minimal mutexes (only for shared state like DMD queue)
- Prefer channels for inter-goroutine communication
- Read-write locks for DMD cached state

## Performance Targets

- **Throughput**: 1000 logs/second
- **Latency (P99)**: <500ms end-to-end
- **Memory**: <100 MB steady-state
- **CPU**: <50% on 2 cores
