# Observer: Correlation Algorithm Design Guide

## Goal

Design correlation algorithms that collect evidence giving an LLM enough context to identify root causes.

---

## Constraints

When designing correlation algorithms, consider:

- **Scale:** Hundreds of unique metric sources, thousands of anomalies
- **Noise:** Many duplicates and false positives
- **Real-time:** Cannot be computationally expensive
- **Output:** Must provide structured evidence for LLM diagnosis

---

## Algorithm Categories

### Pre-Correlation Filtering

Reduce anomaly volume before correlation to:
- Prevent overwhelming the correlator
- Remove obvious duplicates
- Focus on significant anomalies

**Approaches:**
- Bloom filter deduplication (space-efficient duplicate detection)
- Severity thresholding (filter low-confidence anomalies)
- Time bucketing (one anomaly per source per time window)

### Temporal Correlation

Group anomalies by time proximity.

**Approaches:**
- Simple time clustering (group within N seconds)
- Lead-lag detection (A consistently precedes B)
- Causal graph discovery (directed edges based on timing)

**Output format:**
```json
{
  "temporal_clusters": [
    {"time": "...", "anomalies": [...]}
  ],
  "causal_edges": [
    {"leader": "A", "follower": "B", "lag_seconds": 5}
  ]
}
```

### Co-occurrence Analysis

Detect patterns in which anomalies appear together.

**Approaches:**
- Lift/support (association rule mining)
- Mutual information
- Correlation coefficients

**Key metrics:**
- `lift(A,B) = P(A and B) / (P(A) * P(B))`
  - lift > 1: co-occur more than expected
  - lift < 1: co-occur less than expected

**Output format:**
```json
{
  "surprising_cooccurrences": [
    {"sources": ["A", "B"], "lift": 4.2}
  ]
}
```

### Graph-Based Methods

Learn relationships between sources over time.

**Approaches:**
- Edge frequency learning (count co-occurrences)
- Graph sketching (probabilistic edge counting)
- Causal graph inference

**Output format:**
```json
{
  "learned_edges": [
    {"source": "A", "target": "B", "strength": 0.85}
  ]
}
```

---

## Design Principles

### 1. Evidence Over Decisions

The correlator's job is to surface evidence, not make diagnoses. Let the LLM interpret the evidence.

**Good:** "A and B co-occurred 12 times with lift 3.8"
**Bad:** "A caused B"

### 2. Structured Output

Provide machine-readable output that the LLM can parse and reason about.

**Good:** JSON with clear field names and types
**Bad:** Free-form text descriptions

### 3. Bounded Memory

Algorithms must handle unbounded streams without memory growth.

**Techniques:**
- Stable Bloom filters (probabilistic with eviction)
- Ring buffers (fixed-size sliding windows)
- Decay factors (old data contributes less)

### 4. Interpretable Parameters

Each parameter should have clear semantic meaning.

**Good:** `window_size_seconds = 10`
**Bad:** `magic_factor = 0.73`

---

## Implementation Pattern

### Correlator Interface

```go
type Correlator interface {
    // Process a single anomaly
    ProcessAnomaly(anomaly Anomaly)

    // Get current correlation state
    GetCorrelations() CorrelationOutput

    // Reset state
    Reset()
}
```

### Configuration Structure

```go
type CorrelatorConfig struct {
    // Common parameters
    WindowSizeSeconds int64
    MinObservations   int

    // Algorithm-specific parameters
    // ...
}

func DefaultConfig() CorrelatorConfig {
    return CorrelatorConfig{
        WindowSizeSeconds: 10,
        MinObservations:   3,
    }
}
```

### Testing Pattern

```go
func TestCorrelator(t *testing.T) {
    // Setup
    c := NewCorrelator(DefaultConfig())

    // Inject known pattern
    c.ProcessAnomaly(Anomaly{Source: "A", Time: 0})
    c.ProcessAnomaly(Anomaly{Source: "B", Time: 5})
    c.ProcessAnomaly(Anomaly{Source: "A", Time: 10})
    c.ProcessAnomaly(Anomaly{Source: "B", Time: 15})

    // Verify detection
    result := c.GetCorrelations()
    assert.Contains(t, result.Edges, Edge{Leader: "A", Follower: "B"})
}
```

---

## Literature References

### Streaming Algorithms
- Stable Bloom Filters (Deng & Rafiei 2006) - Probabilistic deduplication for unbounded streams

### Causal Discovery
- Transfer entropy methods - Information-theoretic causality
- Granger causality - Time series prediction-based causality

### Association Mining
- Lift/support metrics - Classic association rule mining
- Count-Min Sketch - Probabilistic frequency counting

### Graph Learning
- Sketching algorithms - Space-efficient graph summaries
- Online learning - Incremental model updates

---

## Verification Checklist

For any new correlator:

- [ ] Memory bounded (no unbounded growth)
- [ ] Parameters documented with ranges
- [ ] Unit tests with known patterns
- [ ] Output format documented
- [ ] Integration with demo CLI
- [ ] Evaluated on train scenarios
