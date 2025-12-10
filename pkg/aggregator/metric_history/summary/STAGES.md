# Anomaly Summary System - Implementation Stages

This document defines the staged implementation plan for the anomaly summary system,
which provides semantic aggregation of detected anomalies with automatic clustering
by time and metric/tag dimensions.

## Overview

The system transforms a stream of individual anomaly events into a live semantic summary
that groups related anomalies and extracts meaningful patterns (e.g., "disk space shifted
across 6 APFS volumes" instead of 11 separate alerts).

---

## Stage 1: Tag Relationship Discovery

**Goal**: Given a set of anomaly events, correctly partition tags into "constant" vs "varying"
and identify the dimension being clustered over.

**Why first**: This is the core insight that makes clustering meaningful. If we get this wrong,
everything else is noise. It's also the least proven—we should validate the approach before
building infrastructure around it.

**Scope**:
- Input: `[]AnomalyEvent` (already grouped, clustering comes later)
- Output: `TagPartition{ConstantTags, VaryingTags}`
- No persistence, no lifecycle, no clustering logic yet

```go
type AnomalyEvent struct {
    Timestamp time.Time
    Metric    string
    Tags      map[string]string
    Severity  float64
    Direction string // "increase" or "decrease"
}

type TagPartition struct {
    ConstantTags map[string]string   // key -> value (same across all events)
    VaryingTags  map[string][]string // key -> distinct values seen
}

func PartitionTags(events []AnomalyEvent) TagPartition
```

**Success criteria**:

1. **Single-dimension variation**: 6 disk events with different `device` values →
   `VaryingTags: {device: [6 values]}`, `ConstantTags: {}` (or host if present on all)

2. **Multi-dimension variation**: Events varying by both `device` and `host` →
   both appear in `VaryingTags`

3. **Mixed constant/varying**: All events have `env:prod` but different `container_id` →
   `ConstantTags: {env: prod}`, `VaryingTags: {container_id: [...]}`

4. **No tags**: Events with empty tags → both maps empty

5. **Single event**: One event → all tags are "constant" (degenerate case)

**Test cases**:
```go
func TestPartitionTags_SingleDimension(t *testing.T)
func TestPartitionTags_MultiDimension(t *testing.T)
func TestPartitionTags_MixedConstantVarying(t *testing.T)
func TestPartitionTags_NoTags(t *testing.T)
func TestPartitionTags_SingleEvent(t *testing.T)
func TestPartitionTags_RealDiskScenario(t *testing.T) // use actual snapshot data patterns
```

**Deliverable**: `tags.go` + `tags_test.go`

---

## Stage 2: Metric Family Extraction

**Goal**: Given a set of anomaly events, extract the common metric prefix and identify
which suffixes vary.

**Why second**: This is the other half of pattern identification. Combined with Stage 1,
we can describe "what changed" semantically.

**Scope**:
- Input: `[]AnomalyEvent`
- Output: `MetricPattern{Family, Variants}`

```go
type MetricPattern struct {
    Family   string   // e.g., "system.disk"
    Variants []string // e.g., ["free", "used"]
}

func ExtractMetricPattern(events []AnomalyEvent) MetricPattern
```

**Success criteria**:

1. **Common prefix**: `[system.disk.free, system.disk.used]` →
   `Family: "system.disk"`, `Variants: ["free", "used"]`

2. **Deeper nesting**: `[system.cpu.user.total, system.cpu.system.total]` →
   `Family: "system.cpu"`, `Variants: ["user.total", "system.total"]`

3. **Single metric**: `[system.load.1, system.load.1, system.load.1]` →
   `Family: "system.load.1"`, `Variants: []`

4. **No common prefix**: `[system.cpu.user, system.disk.free]` →
   `Family: "system"`, `Variants: ["cpu.user", "disk.free"]`

5. **Identical metrics**: All same metric name →
   `Family: <that metric>`, `Variants: []`

**Test cases**:
```go
func TestExtractMetricPattern_CommonPrefix(t *testing.T)
func TestExtractMetricPattern_DeeperNesting(t *testing.T)
func TestExtractMetricPattern_SingleMetric(t *testing.T)
func TestExtractMetricPattern_NoCommonPrefix(t *testing.T)
func TestExtractMetricPattern_RealDiskScenario(t *testing.T)
```

**Deliverable**: `metrics.go` + `metrics_test.go`

---

## Stage 3: Basic Clustering

**Goal**: Group incoming anomaly events into clusters based on time proximity and
tag/metric compatibility.

**Why third**: Now we have the building blocks (tag partitioning, metric patterns) to
define what makes events "similar." This stage wires them into incremental clustering.

**Scope**:
- Incremental insertion of events
- Clusters formed when 2+ events are compatible
- No lifecycle (active/resolved), no expiration yet
- Compatibility based on: time window + metric family + tag key overlap

```go
type AnomalyCluster struct {
    ID      int
    Events  []AnomalyEvent
    Pattern *ClusterPattern // combines MetricPattern + TagPartition
}

type ClusterPattern struct {
    MetricPattern
    TagPartition
}

type ClusterSet struct {
    clusters map[int]*AnomalyCluster
    pending  []AnomalyEvent // unclustered events
}

func NewClusterSet(cfg ClusterConfig) *ClusterSet
func (cs *ClusterSet) Add(event AnomalyEvent)
func (cs *ClusterSet) Clusters() []*AnomalyCluster
func (cs *ClusterSet) Pending() []AnomalyEvent
```

**Success criteria**:

1. **Time-adjacent same-metric events cluster**: Two `system.disk.free` events 5s apart →
   same cluster

2. **Time-adjacent different-metric same-family cluster**: `system.disk.free` and
   `system.disk.used` 5s apart → same cluster

3. **Time-distant events don't cluster**: Same metric, 5 minutes apart →
   separate clusters (or pending)

4. **Different families don't cluster**: `system.disk.free` and `system.cpu.user` same time →
   separate

5. **Tag compatibility matters**: Events with disjoint tag keys don't cluster

6. **Pattern updates on add**: Adding 3rd event to cluster updates `TagPartition` correctly

**Test cases**:
```go
func TestClusterSet_TimeAdjacent(t *testing.T)
func TestClusterSet_SameFamily(t *testing.T)
func TestClusterSet_TimeDistant(t *testing.T)
func TestClusterSet_DifferentFamilies(t *testing.T)
func TestClusterSet_TagCompatibility(t *testing.T)
func TestClusterSet_PatternUpdates(t *testing.T)
func TestClusterSet_RealDiskScenario(t *testing.T) // 6 disk events → 1 cluster
```

**Deliverable**: `cluster.go` + `cluster_test.go`

---

## Stage 4: Symmetry Detection

**Goal**: Detect when metrics in a cluster have inverse or proportional relationships
(e.g., `disk.free↑` = `disk.used↓`).

**Why fourth**: This is what turns "6 anomalies" into "disk space shifted." High-value
semantic extraction that builds on clustering.

**Scope**:
- Analyze events within a cluster
- Detect inverse pairs (opposite direction, similar magnitude)
- Detect proportional changes (same direction, correlated magnitude)

```go
type SymmetryPattern struct {
    Type       SymmetryType // Inverse, Proportional, None
    Metrics    [2]string    // the two metrics involved
    Confidence float64      // 0-1
}

type SymmetryType int
const (
    NoSymmetry SymmetryType = iota
    Inverse                  // free↑ = used↓
    Proportional             // read↑ ~ write↑
)

func DetectSymmetry(events []AnomalyEvent) *SymmetryPattern
```

**Success criteria**:

1. **Inverse detection**: `disk.free +12MB` and `disk.used -12MB` at same time →
   `Inverse` with high confidence

2. **Magnitude tolerance**: `disk.free +12MB` and `disk.used -11MB` →
   still `Inverse` (within tolerance)

3. **No false positives**: `disk.free +12MB` and `cpu.user +5%` → `NoSymmetry`

4. **Proportional detection**: `io.read +100` and `io.write +95` same direction →
   `Proportional`

5. **Multi-event robustness**: Pattern holds across multiple timestamp groups in cluster

**Test cases**:
```go
func TestDetectSymmetry_Inverse(t *testing.T)
func TestDetectSymmetry_MagnitudeTolerance(t *testing.T)
func TestDetectSymmetry_NoFalsePositives(t *testing.T)
func TestDetectSymmetry_Proportional(t *testing.T)
func TestDetectSymmetry_MultiEvent(t *testing.T)
func TestDetectSymmetry_RealDiskScenario(t *testing.T)
```

**Deliverable**: `symmetry.go` + `symmetry_test.go`

---

## Stage 5: Summary Generation

**Goal**: Generate human-readable summaries from cluster patterns.

**Why fifth**: All the semantic extraction is done; now we render it. Separating this
allows tuning output format without touching analysis logic.

**Scope**:
- Input: `*AnomalyCluster` (with Pattern, Symmetry)
- Output: Structured summary text
- Include likely cause heuristics

```go
type ClusterSummary struct {
    Headline    string   // "Disk space shift across 6 APFS volumes"
    Details     []string // bullet points
    LikelyCause string   // heuristic guess, may be empty
}

func Summarize(cluster *AnomalyCluster) ClusterSummary
```

**Success criteria**:

1. **Varying dimension in headline**: 6 devices → "across 6 devices"

2. **Symmetry in details**: Inverse pattern → "Pattern: free↑ = used↓"

3. **Constant tags shown**: `env:prod` constant → mentioned in details

4. **Likely cause for known patterns**: APFS multi-volume → suggests snapshot/reclamation

5. **Graceful degradation**: Unknown pattern → no likely cause, still readable summary

**Test cases**:
```go
func TestSummarize_VaryingDimension(t *testing.T)
func TestSummarize_SymmetryIncluded(t *testing.T)
func TestSummarize_ConstantTags(t *testing.T)
func TestSummarize_LikelyCause(t *testing.T)
func TestSummarize_UnknownPattern(t *testing.T)
func TestSummarize_RealDiskScenario(t *testing.T)
```

**Deliverable**: `render.go` + `render_test.go`

---

## Stage 6: Lifecycle & Expiration

**Goal**: Manage cluster state over time (Active → Stabilizing → Resolved → expired).

**Why sixth**: Now we have working clustering and summarization. This stage makes it
production-ready with proper resource management.

**Scope**:
- State transitions based on time since last event
- Expiration of old clusters
- Expiration of stale pending events
- `Tick()` method called periodically

```go
type ClusterState int
const (
    Active ClusterState = iota
    Stabilizing
    Resolved
)

func (cs *ClusterSet) Tick(now time.Time) // update states, expire old data
func (c *AnomalyCluster) State() ClusterState
```

**Success criteria**:

1. **Active → Stabilizing**: No events for 30s → state changes

2. **Stabilizing → Resolved**: No events for 2min → state changes

3. **Resolved expiration**: No events for 10min → cluster removed

4. **Pending expiration**: Unclustered event older than 2min → removed

5. **State shown in summary**: Resolved clusters marked as such

**Test cases**:
```go
func TestLifecycle_ActiveToStabilizing(t *testing.T)
func TestLifecycle_StabilizingToResolved(t *testing.T)
func TestLifecycle_ResolvedExpiration(t *testing.T)
func TestLifecycle_PendingExpiration(t *testing.T)
func TestLifecycle_StateInSummary(t *testing.T)
```

**Deliverable**: Updates to `cluster.go` + lifecycle tests

---

## Stage 7: Integration

**Goal**: Wire the summary system into the existing anomaly detection pipeline.

**Why last**: All components are tested in isolation. This stage connects them to the
real system.

**Scope**:
- Create `AnomalySummary` in demultiplexer alongside cache
- Feed anomalies from `OnFlush()` into summary
- Call `Tick()` on flush cycle
- Log summary output (replacing per-anomaly logging)

**Success criteria**:

1. **Demo produces grouped output**: Running demo script shows cluster summary instead
   of 11 separate lines

2. **No regression**: All anomalies still detected (just grouped)

3. **Performance acceptable**: Summary overhead < 1ms per flush with typical anomaly rates

**Test cases**:
```go
func TestIntegration_DemoScenario(t *testing.T) // use snapshot data
func TestIntegration_HighThroughput(t *testing.T) // benchmark
```

**Deliverable**: Updates to `demultiplexer_agent.go`, integration test

---

## Summary Table

| Stage | Component | Risk | Complexity | Success Metric |
|-------|-----------|------|------------|----------------|
| 1 | Tag Partitioning | **High** (novel) | Low | Correct constant/varying split |
| 2 | Metric Patterns | Medium | Low | Correct family/variant extraction |
| 3 | Basic Clustering | Medium | Medium | Events grouped correctly |
| 4 | Symmetry Detection | Medium | Medium | Inverse relationships found |
| 5 | Summary Generation | Low | Low | Readable output |
| 6 | Lifecycle | Low | Medium | Proper state transitions |
| 7 | Integration | Low | Medium | End-to-end demo works |

Stages 1-2 can be done in parallel. Stages 3-5 are sequential. Stage 6 can be done
in parallel with Stage 5. Stage 7 depends on all others.

---

## Implementation Process

Each stage follows this pattern:

1. **Test Writer**: Create tests representing success criteria with realistic test data
   and a stub implementation that defines the interface

2. **Implementer**: Implement functionality to pass tests, keeping code simple and
   avoiding mixing concerns across stages

3. **Reviewer**: Review tests and implementation for correctness and representative
   quality (prototype-grade, not production-grade)

4. **Fix Issues**: Address any issues found in review

Each step commits its work to git for history tracking.
