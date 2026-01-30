# Observer: New Correlation Approaches for LLM Diagnosis

## Goal
Collect evidence that gives an LLM enough context to identify what went wrong.

## Constraints
- **Scale**: 200+ unique sources, thousands of anomalies
- **Noise**: Many duplicates and false positives
- **Real-time**: Cannot be resource-intensive
- **Approach**: Start simple (dedup + temporal), also implement surprise/lift for comparison

## Current Pain Points
- Network latency scenarios fail (<10/100)
- Pure co-occurrence doesn't discriminate when everything co-occurs
- No deduplication - thousands of anomalies overwhelm correlation

---

## Implementation Plan

### Phase 1: Stable Bloom Filter Deduplication

**Goal:** Reduce anomaly volume, fix backpressure

**File:** `comp/observer/impl/anomaly_dedup.go`

**Literature:** Deng & Rafiei (2006) - "Approximately Detecting Duplicates for Streaming Data using Stable Bloom Filters"

**Custom Implementation (no external libraries):**

```go
package observerimpl

import (
    "hash/fnv"
    "math/rand"
    "sync"
)

// StableBloomFilter handles unbounded streams by probabilistically evicting old entries.
// Unlike classic Bloom filters that fill up, this maintains a stable false positive rate.
type StableBloomFilter struct {
    cells     []uint8  // Each cell is a counter (0-max)
    numCells  uint32
    numHashes uint32
    max       uint8    // Max counter value (e.g., 3)
    p         uint32   // Number of cells to decrement on each add (controls eviction rate)
    mu        sync.RWMutex
    rng       *rand.Rand
}

// NewStableBloomFilter creates a new Stable Bloom Filter.
// - numCells: size of the filter (larger = lower FP rate)
// - numHashes: number of hash functions (typically 3-5)
// - max: maximum counter value (typically 3)
// - p: cells to decrement per add (controls memory/accuracy tradeoff)
func NewStableBloomFilter(numCells, numHashes uint32, max uint8, p uint32) *StableBloomFilter {
    return &StableBloomFilter{
        cells:     make([]uint8, numCells),
        numCells:  numCells,
        numHashes: numHashes,
        max:       max,
        p:         p,
        rng:       rand.New(rand.NewSource(42)),
    }
}

// hash returns k hash values for the given key
func (f *StableBloomFilter) hash(key []byte) []uint32 {
    hashes := make([]uint32, f.numHashes)
    h1 := fnv.New32a()
    h1.Write(key)
    hash1 := h1.Sum32()

    h2 := fnv.New32()
    h2.Write(key)
    hash2 := h2.Sum32()

    // Double hashing: h(i) = h1 + i*h2
    for i := uint32(0); i < f.numHashes; i++ {
        hashes[i] = (hash1 + i*hash2) % f.numCells
    }
    return hashes
}

// Add inserts an element and randomly decrements p cells (eviction)
func (f *StableBloomFilter) Add(key []byte) {
    f.mu.Lock()
    defer f.mu.Unlock()

    // Decrement p random cells (stable eviction)
    for i := uint32(0); i < f.p; i++ {
        idx := f.rng.Uint32() % f.numCells
        if f.cells[idx] > 0 {
            f.cells[idx]--
        }
    }

    // Set cells for this key to max
    for _, idx := range f.hash(key) {
        f.cells[idx] = f.max
    }
}

// Test checks if an element might be in the filter
func (f *StableBloomFilter) Test(key []byte) bool {
    f.mu.RLock()
    defer f.mu.RUnlock()

    for _, idx := range f.hash(key) {
        if f.cells[idx] == 0 {
            return false  // Definitely not present
        }
    }
    return true  // Possibly present
}

// TestAndAdd atomically tests and adds
func (f *StableBloomFilter) TestAndAdd(key []byte) bool {
    f.mu.Lock()
    defer f.mu.Unlock()

    // Test first
    present := true
    for _, idx := range f.hash(key) {
        if f.cells[idx] == 0 {
            present = false
            break
        }
    }

    // Decrement p random cells
    for i := uint32(0); i < f.p; i++ {
        idx := f.rng.Uint32() % f.numCells
        if f.cells[idx] > 0 {
            f.cells[idx]--
        }
    }

    // Add
    for _, idx := range f.hash(key) {
        f.cells[idx] = f.max
    }

    return present
}

// AnomalyDeduplicator wraps StableBloomFilter for anomaly deduplication
type AnomalyDeduplicator struct {
    filter            *StableBloomFilter
    bucketSizeSeconds int64
}

func NewAnomalyDeduplicator(numCells uint32, bucketSize int64) *AnomalyDeduplicator {
    return &AnomalyDeduplicator{
        // numCells=100000, numHashes=3, max=3, p=1
        filter:            NewStableBloomFilter(numCells, 3, 3, 1),
        bucketSizeSeconds: bucketSize,
    }
}

func (d *AnomalyDeduplicator) ShouldProcess(source string, timestamp int64) bool {
    key := fmt.Sprintf("%s|%d", source, timestamp/d.bucketSizeSeconds)
    return !d.filter.TestAndAdd([]byte(key))
}
```

**Parameters:**
- `numCells`: 100,000 (~100KB memory)
- `numHashes`: 3
- `max`: 3 (counter ceiling)
- `p`: 1 (cells to decrement per add)
- `bucketSizeSeconds`: 5

---

### Phase 2A: Temporal Lead-Lag Correlator (Simple)

**Goal:** Detect "A leads B by N seconds" patterns for root cause evidence

**File:** `comp/observer/impl/anomaly_processor_leadlag.go`

**Literature:** [RADICE 2025](https://arxiv.org/html/2501.11545v1) - "causal graph discovery for root cause analysis"

```go
type LeadLagCorrelator struct {
    // Recent anomaly timestamps per source (ring buffer)
    sourceTimestamps map[string]*RingBuffer  // last 100 timestamps per source

    // Lag histograms for source pairs
    lagHistograms map[string]*LagHistogram  // "A|B" -> histogram

    // Configuration
    maxLagSeconds   int64  // e.g., 30s
    minObservations int    // e.g., 3

    mu sync.RWMutex
}

type LagHistogram struct {
    // Bins for lags: [-30, -20, -10, -5, 0, +5, +10, +20, +30] seconds
    bins              [9]int
    totalObservations int
}

type LeadLagEdge struct {
    Leader       string  // Source that leads
    Follower     string  // Source that follows
    TypicalLag   int64   // Seconds
    Confidence   float64 // Consistency of lag direction
    Observations int
}
```

**Algorithm:**
1. On new anomaly for source A at time T:
   - For each other source B with recent anomalies:
     - Compute lag = T - B_most_recent_time
     - Update lag histogram for pair (A, B)
2. When queried for correlations:
   - For each pair with enough observations:
     - Check if lag distribution is skewed (one source consistently leads)
     - Report edges where confidence > 0.6

**Output for LLM:**
```json
{
  "temporal_evidence": [
    {
      "leader": "network.retransmits:avg",
      "follower": "connection.errors:count",
      "typical_lag_seconds": 5,
      "confidence": 0.82,
      "observations": 12
    }
  ]
}
```

---

### Phase 2B: Surprise/Lift Correlator (Compare)

**Goal:** Detect unexpected co-occurrences using lift (PMI-like)

**File:** `comp/observer/impl/anomaly_processor_surprise.go`

**Literature:** [Association Rule Mining](https://users.cs.utah.edu/~jeffp/teaching/cs5140-S16/cs5140/L12-Count-Min+Apriori.pdf)

```go
type SurpriseCorrelator struct {
    // Marginal counts per source (how often each source anomalies)
    sourceCounts map[string]int  // Could use Count-Min Sketch for 200+ sources

    // Joint counts for pairs (how often A and B co-occur)
    pairCounts map[string]int   // "A|B" -> count

    // Total time windows observed
    totalWindows int

    // Current window tracking
    currentWindowStart   int64
    windowSizeSeconds    int64  // e.g., 10s
    currentWindowSources map[string]bool

    mu sync.RWMutex
}

type SurpriseEdge struct {
    Source1      string
    Source2      string
    Lift         float64  // > 1 = surprising co-occurrence
    Support      float64  // How often they co-occur
    Observations int
}
```

**Algorithm:**
```
lift(A, B) = P(A ∩ B) / (P(A) × P(B))
           = pairCount[A,B] × totalWindows / (sourceCount[A] × sourceCount[B])
```

- `lift > 2.0`: A and B co-occur MORE than expected → interesting pattern
- `lift < 0.5`: A and B co-occur LESS than expected → also interesting (anti-correlation)
- `lift ≈ 1.0`: Independent, just random co-occurrence

**Key insight:** When everything co-occurs, lift helps discriminate:
- High lift = surprisingly together (unusual pattern)
- Low lift = surprisingly NOT together (expected pattern missing)

**Output for LLM:**
```json
{
  "surprising_cooccurrences": [
    {
      "sources": ["network.retransmits:avg", "disk.io_wait:avg"],
      "lift": 4.2,
      "note": "These rarely co-occur, but did 8 times this window"
    }
  ],
  "expected_but_rare": [
    {
      "sources": ["cpu.usage:avg", "memory.usage:avg"],
      "lift": 0.3,
      "note": "Usually co-occur, but didn't this time - unusual"
    }
  ]
}
```

---

### Phase 3: Combined Evidence Output

**File:** `comp/observer/impl/evidence_aggregator.go`

```go
type EvidencePackage struct {
    // From LeadLagCorrelator
    TemporalChain []LeadLagEdge  // "A → B → C with lags"

    // From SurpriseCorrelator
    SurprisingPatterns []SurpriseEdge  // High lift
    MissingPatterns    []SurpriseEdge  // Low lift (expected but didn't happen)

    // Anomaly details
    Anomalies []AnomalyWithSeverity

    // Summary for LLM
    PatternDescription string
}
```

**Example combined output:**
```json
{
  "cluster_id": "c_1706000042",
  "temporal_chain": [
    {"leader": "network.retransmits", "follower": "connection.errors", "lag": 5},
    {"leader": "connection.errors", "follower": "app.latency", "lag": 3}
  ],
  "surprising_patterns": [
    {"sources": ["disk.io_wait", "network.retransmits"], "lift": 3.8}
  ],
  "missing_patterns": [
    {"sources": ["cpu.throttle", "memory.pressure"], "lift": 0.2,
     "note": "Usually co-occur but CPU throttle didn't happen"}
  ],
  "anomalies": [
    {"source": "network.retransmits", "severity": 3.2},
    {"source": "connection.errors", "severity": 2.1},
    {"source": "app.latency", "severity": 1.8}
  ],
  "pattern_description": "Network retransmits led cascade to connection errors (+5s) then app latency (+3s). Unusually, disk IO wait also spiked (lift=3.8)."
}
```

---

## Files to Create/Modify

**Create:**
- `comp/observer/impl/anomaly_dedup.go` - Stable Bloom Filter dedup
- `comp/observer/impl/anomaly_processor_leadlag.go` - Temporal lead-lag
- `comp/observer/impl/anomaly_processor_surprise.go` - Lift/surprise correlation
- `comp/observer/impl/evidence_aggregator.go` - Combined output

**Modify:**
- `comp/observer/impl/observer.go` - Integrate new components
- `comp/observer/def/types.go` - Add new output types
- `comp/observer/impl/observeropts.go` - Add config options to enable/disable each correlator

---

## Verification Plan

1. **Unit tests:**
   - Dedup: Verify duplicates filtered, unique anomalies pass through
   - Lead-Lag: Inject known A→B pattern, verify detection
   - Surprise: Inject rare co-occurrence, verify high lift

2. **Comparison test:**
   - Run both correlators on same scenario
   - Compare which provides more useful evidence for network-latency case

3. **Scale test:**
   - 200+ sources, 1000+ anomalies
   - Verify memory bounded, latency acceptable

4. **LLM integration test:**
   - Feed evidence to LLM
   - Evaluate diagnosis quality

---

## Literature References

- [Stable Bloom Filters (Deng & Rafiei 2006)](https://webdocs.cs.ualberta.ca/~drafiei/papers/DupDet06Sigmod.pdf) - Original SBF paper for streaming dedup
- [RADICE 2025](https://arxiv.org/html/2501.11545v1) - Causal graph discovery for RCA
- [CGAD (ACM TIST 2024)](https://dl.acm.org/doi/10.1145/3757922) - Transfer entropy for causal graphs
- [Concept Drift Survey (Frontiers 2024)](https://www.frontiersin.org/journals/artificial-intelligence/articles/10.3389/frai.2024.1330257/full) - Gradual drift detection
- [Association Rule Mining](https://users.cs.utah.edu/~jeffp/teaching/cs5140-S16/cs5140/L12-Count-Min+Apriori.pdf) - Lift/support for pattern mining
