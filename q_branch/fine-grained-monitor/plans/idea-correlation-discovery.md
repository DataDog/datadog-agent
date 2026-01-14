# Correlation Discovery Study - Implementation Plan

## Overview

Implement a **Correlation Discovery** study that automatically finds interesting relationships between metrics and containers. Rather than requiring users to specify two series to correlate, this acts as a discovery mechanism that surfaces hidden patterns like "these 3 containers always spike together" or "memory pressure events coincide with I/O wait spikes."

## Discovery Approaches

### 1. **Top-K Correlation Discovery** (Recommended for MVP)

**Algorithm:**
Given a reference series (metric + container), compute correlation against ALL other series and return the top K strongest correlations.

**Example query:** "What correlates most strongly with CPU usage in pod-frontend?"

**Process:**
1. User selects a metric + container as the "anchor"
2. System loads that series
3. For each other metric × container combination:
   - Load timeseries
   - Compute correlation (with lag detection)
   - Track top-K by absolute correlation value
4. Return ranked list of correlations

**Complexity:** O(N × M) where N = metrics, M = containers
- For 50 metrics × 10 containers = 500 correlations to compute
- With 1000 points each, ~500K correlation computations
- Feasible with optimizations (downsample, parallel compute)

**Output:**
```json
{
  "anchor": { "metric": "cpu_usage", "container": "pod-frontend" },
  "top_correlations": [
    {
      "metric": "memory_pressure_psi",
      "container": "pod-frontend",
      "correlation": 0.92,
      "lag_seconds": -2,
      "p_value": 0.001
    },
    {
      "metric": "cpu_usage",
      "container": "pod-backend",
      "correlation": 0.87,
      "lag_seconds": 0,
      "p_value": 0.002
    },
    ...
  ]
}
```

### 2. **Anomaly-Triggered Correlation Search**

**Algorithm:**
When a changepoint/anomaly is detected in a series, automatically find what else changed around that time.

**Example:** "Memory spiked at 14:32:15 - what else happened?"

**Process:**
1. Detect changepoint in anchor series at time T
2. Define correlation window: [T-30s, T+30s]
3. For each other series:
   - Compute correlation within the window
   - Check for changepoints in the same window
   - Detect temporal proximity of events
4. Return series with:
   - High correlation in the window
   - Changepoints within ±10s of anchor changepoint

**Advantage:** Focused search around specific events, reduces false positives

**Integration:** Extend Changepoint study to optionally include "What else changed?" section

### 3. **Correlation Clustering**

**Algorithm:**
Group metrics/containers by correlation similarity using hierarchical clustering.

**Example:** "Which containers behave similarly?"

**Process:**
1. Compute correlation matrix for all series pairs
2. Convert correlation to distance: `distance = 1 - |correlation|`
3. Apply hierarchical clustering (e.g., Ward's method)
4. Return dendrogram or flat clusters

**Output:**
- "Cluster 1: pod-frontend, pod-backend, pod-api (CPU usage moves together)"
- "Cluster 2: postgres, redis (memory pressure correlated)"

**Use case:** Identify resource contention groups, redundant metrics

### 4. **Time-Window Discovery**

**Algorithm:**
For a specific time range (especially around incidents), compute all-pairs correlation and surface strongest relationships.

**Example:** "During the outage (14:00-15:00), what metrics were related?"

**Process:**
1. User specifies time window
2. Compute all-pairs correlation within that window only
3. Return top-K strongest correlations
4. Optional: Compare to baseline correlation (full dataset) to find anomalous relationships

**Advantage:** Incident-specific insights, avoids steady-state correlations

## Recommended MVP: Top-K Discovery

**Why this approach:**
1. **User-friendly:** Simple mental model - "show me what correlates with X"
2. **Actionable:** Returns ranked, specific findings
3. **Performant:** Can optimize with early termination, caching
4. **Extensible:** Easy to add filters (same metric only, cross-container only, etc.)

**API Design:**
```
GET /api/discover-correlations
  ?anchor_metric=cpu_usage
  &anchor_container=pod-frontend
  &top_k=10
  &range=1h
  &min_correlation=0.5
  &include_same_metric=false  // Skip cpu_usage in other containers
```

**UI Integration:**
- Add button next to any chart: "Find correlations"
- Opens modal/panel showing top correlations
- Click a correlation to add it to a new chart panel for side-by-side view

## Key Challenges

### 1. Computational Performance

**Challenge:** Computing N×M correlations (50 metrics × 10 containers = 500 pairs)

**Optimizations:**
1. **Downsampling:** Reduce to max 500-1000 points before correlation
   - Use LTTB (Largest-Triangle-Three-Buckets) algorithm for visual-preserving downsampling
2. **Early termination:** Track running top-K, skip series that can't beat the Kth best
3. **Caching:** Cache correlation results per time range
4. **Parallel computation:** Use rayon to parallelize correlation computations
5. **Bloom filter:** Skip containers with no data overlap

**Target:** <2 seconds for 500 series × 1000 points each

### 2. Time Alignment

**Challenge:**
- Metrics may have slightly different timestamps (different collection paths)
- Need to align timeseries before computing correlation

**Approach:**
- Use time-based bucketing (e.g., 1-second bins)
- Interpolate missing values or skip misaligned points
- Require minimum overlap threshold (e.g., 50% of points must align)

### 3. Lag Detection (Cross-Correlation)

**Challenge:**
- One metric may lead or lag another (e.g., "GC events lead CPU spikes by 2 seconds")
- Standard Pearson correlation only handles zero-lag

**Approach:**
- For top-K candidates, compute correlation at multiple lags: [-10s, +10s]
- Return the lag with maximum absolute correlation
- **Optimization:** Only compute lag for promising candidates (|r| > 0.5 at lag=0)
- This enables causality detection while keeping performance reasonable

## Implementation Plan - Top-K Discovery

### Phase 1: Core Correlation Discovery Logic

**File:** `src/metrics_viewer/studies/correlation_discovery.rs`

**Components:**

1. **CorrelationDiscovery struct**
   ```rust
   pub struct CorrelationDiscovery {
       config: CorrelationConfig,
   }

   pub struct CorrelationConfig {
       pub top_k: usize,                 // Number of top correlations to return (default: 10)
       pub min_overlap: usize,           // Min shared points (default: 50)
       pub min_correlation: f64,          // Min |r| to report (default: 0.3)
       pub max_lag_seconds: i64,          // Max lag to test (default: 10)
       pub enable_lag_detection: bool,    // Compute cross-correlation (slower)
       pub downsample_threshold: usize,   // Downsample if > N points (default: 1000)
   }
   ```

2. **Top-K tracking structure**
   ```rust
   struct CorrelationCandidate {
       metric_name: String,
       container_id: String,
       correlation: f64,
       lag_seconds: i64,
       p_value: f64,
       n_samples: usize,
   }

   struct TopKTracker {
       candidates: BinaryHeap<CorrelationCandidate>,  // Max-heap by |correlation|
       k: usize,
   }
   ```

3. **Main discovery function**
   ```rust
   impl CorrelationDiscovery {
       pub fn discover_correlations(
           &self,
           anchor_series: &[TimeseriesPoint],
           all_series: HashMap<SeriesKey, Vec<TimeseriesPoint>>,
       ) -> Vec<CorrelationCandidate> {
           let mut tracker = TopKTracker::new(self.config.top_k);

           // Downsample anchor if needed
           let anchor = downsample_if_needed(anchor_series, self.config.downsample_threshold);

           // Compute correlation against all series in parallel
           all_series.par_iter().for_each(|(key, series)| {
               let candidate_series = downsample_if_needed(series, self.config.downsample_threshold);

               // Align timeseries
               let (aligned_anchor, aligned_candidate) = align_timeseries(&anchor, &candidate_series);

               if aligned_anchor.len() < self.config.min_overlap {
                   return; // Skip insufficient overlap
               }

               // Compute zero-lag correlation first
               let r = pearson_correlation(&aligned_anchor, &aligned_candidate);

               let (best_r, best_lag) = if self.config.enable_lag_detection && r.abs() > 0.5 {
                   // Only compute lag for promising candidates
                   find_best_lag(&aligned_anchor, &aligned_candidate, self.config.max_lag_seconds)
               } else {
                   (r, 0)
               };

               if best_r.abs() >= self.config.min_correlation {
                   let candidate = CorrelationCandidate {
                       metric_name: key.metric.clone(),
                       container_id: key.container.clone(),
                       correlation: best_r,
                       lag_seconds: best_lag,
                       p_value: fisher_z_test(best_r, aligned_anchor.len()),
                       n_samples: aligned_anchor.len(),
                   };

                   tracker.insert(candidate);
               }
           });

           tracker.get_top_k()
       }
   }
   ```

4. **Core algorithm functions**
   ```rust
   fn downsample_if_needed(series: &[TimeseriesPoint], threshold: usize) -> Vec<TimeseriesPoint>
       // LTTB (Largest-Triangle-Three-Buckets) algorithm

   fn align_timeseries(a: &[TimeseriesPoint], b: &[TimeseriesPoint]) -> (Vec<f64>, Vec<f64>)
       // Time-bucket alignment with 1-second bins

   fn pearson_correlation(x: &[f64], y: &[f64]) -> f64
       // Standard Pearson r

   fn find_best_lag(x: &[f64], y: &[f64], max_lag: i64) -> (f64, i64)
       // Cross-correlation over [-max_lag, +max_lag]
       // Returns (best_correlation, best_lag_seconds)

   fn fisher_z_test(r: f64, n: usize) -> f64
       // Approximate p-value for correlation significance
   ```

### Phase 2: API Endpoint for Discovery

**File:** `src/metrics_viewer/server.rs`

**New endpoint:** `GET /api/discover-correlations`

**Query parameters:**
- `anchor_metric` - Metric name for the anchor series
- `anchor_container` - Container ID for the anchor series
- `top_k` - Number of top correlations to return (default: 10)
- `range` - Time range (e.g., "1h")
- `min_correlation` - Minimum |r| to report (default: 0.3)
- `include_same_metric` - Include same metric in other containers (default: true)
- `enable_lag` - Enable lag detection (default: false, slower)

**Handler flow:**
```rust
#[derive(Deserialize)]
struct DiscoverCorrelationsParams {
    anchor_metric: String,
    anchor_container: String,
    #[serde(default = "default_top_k")]
    top_k: usize,
    #[serde(default = "default_range")]
    range: String,
    #[serde(default = "default_min_correlation")]
    min_correlation: f64,
    #[serde(default = "default_true")]
    include_same_metric: bool,
    #[serde(default)]
    enable_lag: bool,
}

#[derive(Serialize)]
struct DiscoverCorrelationsResponse {
    anchor: SeriesKey,
    top_correlations: Vec<CorrelationResult>,
    computation_time_ms: u64,
}

#[derive(Serialize)]
struct CorrelationResult {
    metric: String,
    container: String,
    correlation: f64,
    lag_seconds: i64,
    p_value: f64,
    n_samples: usize,
}

async fn discover_correlations_handler(
    State(state): State<AppState>,
    Query(params): Query<DiscoverCorrelationsParams>,
) -> Result<Json<DiscoverCorrelationsResponse>, StatusCode> {
    let start = Instant::now();

    // 1. Load anchor series
    let time_range = parse_time_range(&params.range)?;
    let anchor_series = state.data.get_timeseries_single(
        &params.anchor_metric,
        &params.anchor_container,
        time_range
    )?;

    // 2. Load ALL available series across all metrics and containers
    let all_series = state.data.get_all_timeseries(time_range)?;

    // 3. Filter out anchor series itself
    all_series.remove(&SeriesKey {
        metric: params.anchor_metric.clone(),
        container: params.anchor_container.clone(),
    });

    // 4. Optionally filter out same metric in other containers
    if !params.include_same_metric {
        all_series.retain(|key, _| key.metric != params.anchor_metric);
    }

    // 5. Run discovery algorithm
    let config = CorrelationConfig {
        top_k: params.top_k,
        min_correlation: params.min_correlation,
        enable_lag_detection: params.enable_lag,
        ..Default::default()
    };

    let discovery = CorrelationDiscovery::new(config);
    let candidates = discovery.discover_correlations(&anchor_series, all_series);

    // 6. Convert to response format
    let results: Vec<CorrelationResult> = candidates.into_iter()
        .map(|c| CorrelationResult {
            metric: c.metric_name,
            container: c.container_id,
            correlation: c.correlation,
            lag_seconds: c.lag_seconds,
            p_value: c.p_value,
            n_samples: c.n_samples,
        })
        .collect();

    Ok(Json(DiscoverCorrelationsResponse {
        anchor: SeriesKey {
            metric: params.anchor_metric,
            container: params.anchor_container,
        },
        top_correlations: results,
        computation_time_ms: start.elapsed().as_millis() as u64,
    }))
}
```

**Required helper in LazyDataStore:**
```rust
impl LazyDataStore {
    // New method to load ALL available series at once
    pub fn get_all_timeseries(
        &self,
        time_range: TimeRange,
    ) -> Result<HashMap<SeriesKey, Vec<TimeseriesPoint>>> {
        // Load all metrics for all containers in the time range
        // Use parallel loading with rayon
    }
}
```

### Phase 3: Frontend Integration

**File:** `src/metrics_viewer/static/js/api-provider.js`

Add new API method:
```javascript
async discoverCorrelations(anchorMetric, anchorContainer, options = {}) {
    const params = new URLSearchParams({
        anchor_metric: anchorMetric,
        anchor_container: anchorContainer,
        top_k: options.topK || 10,
        range: options.range || '1h',
        min_correlation: options.minCorrelation || 0.3,
        include_same_metric: options.includeSameMetric !== false,
        enable_lag: options.enableLag || false,
    });

    const response = await fetch(`/api/discover-correlations?${params}`);
    return await response.json();
}
```

**File:** `src/metrics_viewer/static/js/ui.js`

**UI Changes:**

1. **Add "Find Correlations" button** next to each chart panel
   - Icon: magnifying glass or network icon
   - Tooltip: "Discover what correlates with this series"

2. **Correlation Discovery Modal/Panel:**
   When button clicked, open modal showing:
   ```
   ┌─────────────────────────────────────────────────────────┐
   │  Finding correlations for:                              │
   │  cpu_usage in pod-frontend (last 1h)                    │
   │                                                          │
   │  Top Correlations:                                      │
   │  ┌────────────────────────────────────────────────────┐ │
   │  │ 1. memory_pressure_psi (pod-frontend)    r=0.92   │ │
   │  │    lag=-2s, p<0.001                     [Add]     │ │
   │  ├────────────────────────────────────────────────────┤ │
   │  │ 2. cpu_usage (pod-backend)               r=0.87   │ │
   │  │    lag=0s, p<0.001                      [Add]     │ │
   │  ├────────────────────────────────────────────────────┤ │
   │  │ 3. io_wait_psi (pod-frontend)            r=0.76   │ │
   │  │    lag=-1s, p=0.002                     [Add]     │ │
   │  └────────────────────────────────────────────────────┘ │
   │                                                          │
   │  [Show All 10] [Close]                                  │
   └─────────────────────────────────────────────────────────┘
   ```

3. **"Add" button behavior:**
   - Clicking [Add] creates a new chart panel below the anchor
   - Adds the correlated series to that panel
   - If lag detected, annotate both charts with lag information

4. **Visual indicators:**
   - Color-code correlation strength (green=strong positive, red=strong negative)
   - Show lag with arrow: "← 2s" or "2s →"
   - Display p-value with asterisks: * p<0.05, ** p<0.01, *** p<0.001

**Visualization approach:**
- Correlated series added to separate panels (not overlaid)
- Panels remain time-synchronized for easy visual comparison
- Optional: Draw connecting line between panels showing correlation strength

### Phase 4: Testing

**File:** `src/metrics_viewer/studies/correlation_discovery.rs` (test module)

**Unit tests:**

1. **`test_top_k_selection`**
   - Generate 20 series with varying correlations (0.1 to 0.9)
   - Set top_k=5
   - Verify only 5 strongest correlations returned
   - Verify they're sorted by |correlation| descending

2. **`test_lag_detection`**
   - Create two identical series with one shifted by 3 seconds
   - Enable lag detection
   - Verify detected lag matches actual shift

3. **`test_downsampling_preserves_correlation`**
   - Create correlated series with 10,000 points
   - Downsample to 1000 points
   - Verify correlation coefficient remains similar (within 0.05)

4. **`test_insufficient_overlap_filtered`**
   - Create anchor series [0-100s]
   - Create candidate series [200-300s] (no overlap)
   - Verify candidate not included in results

5. **`test_parallel_computation`**
   - Generate 100 candidate series
   - Verify results identical to sequential computation
   - Verify computation time < sequential time (shows parallelism working)

6. **`test_top_k_tracker`**
   - Insert 100 candidates into TopKTracker with k=10
   - Verify heap maintains only top 10
   - Verify correct ordering

**Integration tests:**

1. **`test_discover_endpoint`**
   - Generate synthetic parquet files with known correlations:
     - Series A: sin wave
     - Series B: same sin wave (r=1.0)
     - Series C: -sin wave (r=-1.0)
     - Series D: random noise (r~0)
   - Call `/api/discover-correlations` with Series A as anchor
   - Verify Series B and C appear in top results
   - Verify Series D does not

2. **`test_discover_cross_container`**
   - Generate data for 3 containers with shared CPU spike pattern
   - Query correlations for container 1
   - Verify containers 2 and 3 appear in results

**Performance benchmark:**
```rust
#[bench]
fn bench_discover_500_series(b: &mut Bencher) {
    // 50 metrics × 10 containers × 1000 points each
    b.iter(|| {
        discovery.discover_correlations(&anchor, &all_series);
    });
    // Target: <2 seconds
}
```

## Decisions

✅ **Discovery Approach:** Implement **Top-K Discovery** where users select an anchor series and the system automatically finds the most correlated series across all metrics and containers.

✅ **API Design:** New `/api/discover-correlations` endpoint with `anchor_metric` and `anchor_container` parameters, returning ranked top-K correlations.

✅ **Scope:** MVP discovers correlations across both same-container (different metrics) and cross-container (same or different metrics). This answers both "What else is happening in this container?" and "Which containers behave similarly?"

✅ **Performance:** Parallel computation with downsampling, early termination, and lazy lag detection (only for promising candidates with |r| > 0.5).

✅ **Statistical significance:** Include p-value computation using Fisher's z-transformation to help filter spurious correlations.

✅ **UI Pattern:** "Find Correlations" button on each chart → Modal with ranked results → Click to add correlated series to new panel.

## Critical Files

### New Files
- `src/metrics_viewer/studies/correlation_discovery.rs` (~400 lines)
  - CorrelationDiscovery struct and config
  - TopKTracker with binary heap
  - Discovery algorithm with parallel computation
  - Downsampling (LTTB), alignment, Pearson correlation
  - Lag detection and p-value computation

### Modified Files
- `src/metrics_viewer/server.rs` (+150 lines)
  - Add `discover_correlations_handler`
  - Add route registration
  - Add request/response structs

- `src/metrics_viewer/lazy_data.rs` (+100 lines)
  - Add `get_all_timeseries()` method
  - Add `get_timeseries_single()` helper
  - Optimize bulk loading with rayon parallelization

- `src/metrics_viewer/static/js/api-provider.js` (+30 lines)
  - Add `discoverCorrelations()` method

- `src/metrics_viewer/static/js/ui.js` (+200 lines)
  - Add "Find Correlations" button to chart panels
  - Implement correlation discovery modal
  - Render ranked correlation results
  - Handle "Add" button to create new panels
  - Visual correlation strength indicators

## Verification Plan

### Manual Testing

1. **Generate synthetic test data:**
   ```bash
   cd /home/bits/go/src/github.com/DataDog/datadog-agent-fgm/q_branch/fine-grained-monitor
   cargo run --release --bin generate-bench-data -- \
     --scenario correlated-metrics \
     --duration 1h \
     --output test-data/
   ```

2. **Start FGM viewer:**
   ```bash
   cargo run --bin fgm-viewer -- ./test-data/ --port 8050
   ```

3. **Open browser to http://localhost:8050**

4. **Test discovery workflow:**
   - Add a chart for `cpu_usage` in `pod-frontend`
   - Click "Find Correlations" button
   - Verify modal shows top correlations
   - Verify correlations are ranked by strength
   - Click "Add" on a correlation
   - Verify new panel created with correlated series
   - Verify panels remain time-synchronized

5. **Test with known patterns:**
   - Synthetic data should have:
     - CPU and memory pressure correlated (r~0.8)
     - Cross-container CPU spikes (r~0.9)
     - I/O and memory uncorrelated (r~0.1)
   - Verify discovery finds expected relationships

### Automated Testing

```bash
# Run all correlation discovery tests
cargo test correlation_discovery

# Run with coverage
cargo tarpaulin --out Html --output-dir coverage -- correlation_discovery

# Run performance benchmarks
cargo bench --bench correlation_bench
```

### API Testing

```bash
# Test discovery endpoint directly
curl "http://localhost:8050/api/discover-correlations?\
anchor_metric=cpu_usage&\
anchor_container=pod-frontend&\
top_k=10&\
range=1h" | jq

# Test with lag detection enabled
curl "http://localhost:8050/api/discover-correlations?\
anchor_metric=memory_current&\
anchor_container=pod-backend&\
enable_lag=true" | jq
```

### MCP Integration (Future)

If/when MCP server exposes correlation discovery:
```bash
# MCP tool: discover_correlations
mcp call discover_correlations \
  --anchor-metric cpu_usage \
  --anchor-container pod-frontend \
  --top-k 5
```

## Implementation Estimate

**Phase 1 (Core discovery logic):** ~400 lines
- CorrelationDiscovery, TopKTracker: ~150 lines
- Core algorithms (align, pearson, lag, downsample): ~150 lines
- Helper functions: ~100 lines

**Phase 2 (API):** ~150 lines
- Handler function: ~80 lines
- Request/response structs: ~40 lines
- LazyDataStore extensions: ~30 lines

**Phase 3 (Frontend):** ~230 lines
- API provider method: ~30 lines
- UI modal and rendering: ~150 lines
- Button and event handlers: ~50 lines

**Phase 4 (Tests):** ~300 lines
- Unit tests: ~150 lines
- Integration tests: ~100 lines
- Benchmarks: ~50 lines

**Total:** ~1,080 lines of new code + modifications to existing files

## Risks & Mitigations

**Risk 1: Performance with large datasets**
- **Challenge:** 50 metrics × 10 containers = 500 series × 1000 points = 500K correlation computations
- **Mitigations:**
  - Downsample all series to max 1000 points using LTTB
  - Parallel computation with rayon (utilize all CPU cores)
  - Lazy lag detection (only for |r| > 0.5)
  - Early termination when top-K heap is full and candidate can't beat Kth best
- **Target:** <2 seconds for 500 series

**Risk 2: False positives (spurious correlations)**
- **Challenge:** With 500 comparisons, expect ~25 false positives at p<0.05
- **Mitigations:**
  - Include p-value in results for user awareness
  - Default min_correlation=0.3 filters weak relationships
  - UI shows confidence indicators (* ** ***)
  - Users can increase min_correlation threshold

**Risk 3: Memory usage loading all series**
- **Challenge:** Loading 50 metrics × 10 containers × 1000 points × 8 bytes = ~4MB uncompressed
- **Mitigations:**
  - LazyDataStore already streams from Parquet (doesn't load all at once)
  - Downsample immediately after loading each series
  - Release original series after downsampling

**Risk 4: UI overwhelm with too many results**
- **Challenge:** Top-10 list might not be enough, or might be too many
- **Mitigations:**
  - Default top_k=10 is reasonable starting point
  - Add "Show All" button to expand list
  - Add search/filter in modal (e.g., "only show cross-container")
  - Color-code by correlation strength for visual scanning

**Risk 5: Lag detection inaccuracy**
- **Challenge:** Time-bucket alignment can introduce ±0.5s error
- **Mitigations:**
  - Use 1-second buckets (matches FGM sampling rate)
  - Report lag in seconds (not milliseconds) to avoid false precision
  - Include lag uncertainty in UI tooltip

**Risk 6: Discovery fatigue**
- **Challenge:** Users might overuse discovery and get analysis paralysis
- **Mitigations:**
  - Don't auto-run discovery (requires explicit button click)
  - Cache results per time range to avoid re-computation
  - Provide clear "actionable insight" messaging (e.g., "Strong correlation suggests shared resource contention")

## Future Enhancements (Post-MVP)

### 1. Anomaly-Triggered Correlation
- Automatically run discovery when changepoint detected
- "Memory spiked at 14:32 - here's what else changed around that time"
- Integration: Extend Changepoint study with "Related Events" section

### 2. Time-Window Discovery
- API: `/api/discover-correlations?time_window=14:00-15:00` (no anchor needed)
- Returns top correlations across ALL series pairs during that window
- Use case: "During the outage, what metrics moved together?"

### 3. Correlation Clustering
- Hierarchical clustering to group similar containers/metrics
- Visualization: Dendrogram showing "pod-frontend, pod-backend, pod-api all behave similarly"
- Use case: Identify redundant metrics, resource contention groups

### 4. Granger Causality
- More sophisticated than correlation: tests if A predicts B
- Statistical test for causality vs mere correlation
- Higher computational cost (requires time-series modeling)

### 5. PCA/Dimensionality Reduction
- Reduce 50 metrics to 3-5 principal components
- Identify latent factors explaining variance
- Use case: "These 10 metrics all measure the same underlying resource pressure"

### 6. Export Correlation Matrix
- Generate full NxN correlation heatmap for all series
- Export as CSV or interactive HTML
- Use case: Offline analysis, sharing with team

### 7. Historical Baseline Comparison
- Compare current correlations to historical baseline
- Detect anomalous relationships: "CPU and memory are more correlated than usual"
- Requires storing past correlation results
