// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package observer provides a component for observing data flowing through the agent.
//
// The observer component allows other components to report metrics, logs, and other
// signals for sampling and analysis. It provides lightweight handles that can be
// passed to data pipelines without adding significant overhead.
package observer

import (
	"sort"
	"strconv"
	"strings"

	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
)

// Handle is the lightweight observation interface passed to other components.
type Handle interface {
	// ObserveMetric observes a DogStatsD metric sample.
	ObserveMetric(sample MetricView)

	// ObserveLog observes a log message.
	ObserveLog(msg LogView)
}

// HandleFunc is a function that returns a handle for a named source.
type HandleFunc func(name string) Handle

// MetricView provides read-only access to a metric sample.
//
// This interface exists to prevent data races. The underlying metric data may be
// reused immediately after ObserveMetric returns, so implementations must not
// store the MetricView itself. Copy any needed values synchronously.
type MetricView interface {
	GetName() string
	GetValue() float64
	GetRawTags() []string
	// GetTimestampUnix returns the sample timestamp in Unix seconds.
	GetTimestampUnix() int64
	GetSampleRate() float64
}

// LogView provides read-only access to a log message.
//
// This interface exists to prevent data races. Implementations must not store
// the LogView itself. Copy any needed values synchronously.
type LogView interface {
	GetContent() string
	GetStatus() string
	Tags() []string
	GetHostname() string
	// GetTimestampUnixMilli returns the agent ingestion timestamp in Unix milliseconds.
	// This is not the log's own timestamp — it reflects when the agent received the log,
	// and is used for internal pipeline latency tracking.
	GetTimestampUnixMilli() int64
}

// LogMetricsExtractor transforms observed logs into metrics.
// Implementations should be fast since they run synchronously on every observed
// log. Extractors may keep lightweight internal state when needed for pattern
// tracking or context enrichment.
type LogMetricsExtractor interface {
	// Name returns the extractor name for debugging and logging.
	Name() string
	// ProcessLog examines a log and returns any derived metrics.
	ProcessLog(log LogView) LogMetricsExtractorOutput
}

// LogObserver is an optional interface that Detectors can implement to
// receive log observations. This allows detectors to incorporate log signals
// without going through the metrics extraction path.
type LogObserver interface {
	Detector
	ProcessLog(log LogView)
}

// MetricOutput is a timeseries value derived from log analysis.
// The storage keeps full summaries (min/max/sum/count) so aggregation
// is specified at read time, not write time.
type MetricOutput struct {
	Name    string
	Value   float64
	Tags    []string
	Context *MetricContext // optional; stored on the series for anomaly enrichment
}

// LogMetricsExtractorOutput is what we obtain when we process a log with a log metrics extractor.
type LogMetricsExtractorOutput struct {
	Metrics   []MetricOutput
	Telemetry []ObserverTelemetry
	// EvictedMetricNames lists metric names whose series should be removed from
	// storage (e.g. after extractor LRU eviction or garbage collection).
	EvictedMetricNames []string
}

// SeriesDescriptor is the fully resolved identity of a time series.
// It carries namespace, metric name, tags, and aggregation — everything
// needed to display, key, and compare series across correlators and API.
type SeriesDescriptor struct {
	// Namespace identifies the component that produced this metric
	// (e.g. an extractor name like "log_metrics_extractor", or "dogstatsd").
	Namespace string
	// Name is the base metric name (e.g. "log.pattern.<hash>.count", "cpu.user").
	Name string
	// Tags are the series-level tags (e.g. ["host:web-1", "env:prod"]).
	Tags []string
	// Aggregate is the aggregation applied when reading the series.
	Aggregate Aggregate
}

// String returns a human-readable representation (e.g. "cpu.user:avg").
// Namespace and tags are not included — use DisplayName() for that.
func (sd SeriesDescriptor) String() string {
	if sd.Name == "" {
		return ""
	}
	if sd.Aggregate == AggregateNone {
		return sd.Name
	}
	return sd.Name + ":" + AggregateString(sd.Aggregate)
}

// DisplayName returns a display string with tags (e.g. "cpu.user:avg{host:web-1}").
func (sd SeriesDescriptor) DisplayName() string {
	base := sd.String()
	if len(sd.Tags) == 0 {
		return base
	}
	return base + "{" + strings.Join(sd.Tags, ",") + "}"
}

// Key returns a stable string suitable for use as a map key.
// Format: "namespace|name:agg|tag1,tag2,..."
func (sd SeriesDescriptor) Key() string {
	aggStr := AggregateString(sd.Aggregate)
	var tagStr string
	if len(sd.Tags) > 0 {
		sorted := make([]string, len(sd.Tags))
		copy(sorted, sd.Tags)
		sort.Strings(sorted)
		tagStr = strings.Join(sorted, ",")
	}
	return sd.Namespace + "|" + sd.Name + ":" + aggStr + "|" + tagStr
}

// SeriesRef is a compact numeric handle for a stored time series.
// Storage assigns a SeriesRef when a series key is first created;
// the ref remains stable for the lifetime of the storage instance.
type SeriesRef int

// QueryHandle pairs a storage series ref with its aggregate, providing
// enough information to produce the compact ID ("42:avg") that the API
// uses as a join key across endpoints.
type QueryHandle struct {
	Ref       SeriesRef
	Aggregate Aggregate
}

// CompactID returns the compact series identifier (e.g. "42:avg").
func (q QueryHandle) CompactID() string {
	return strconv.Itoa(int(q.Ref)) + ":" + AggregateString(q.Aggregate)
}

// AnomalyType distinguishes the source type of an anomaly.
type AnomalyType string

const (
	// AnomalyTypeMetric is a metric-based anomaly detected by a detector.
	AnomalyTypeMetric AnomalyType = "metric"
	// AnomalyTypeLog is a log-based anomaly emitted directly by a log observer,
	// bypassing the metrics extraction pipeline.
	AnomalyTypeLog AnomalyType = "log"
)

// Anomaly is a detected anomaly event.
// Anomalies represent a point in time where something anomalous was detected.
type Anomaly struct {
	// Type distinguishes log-based anomalies from metric-based ones.
	// Defaults to AnomalyTypeMetric if not set.
	Type AnomalyType
	// Source is the fully resolved series identity (namespace, name, tags, aggregate).
	Source SeriesDescriptor
	// SourceRef is the storage handle for this anomaly's series, enabling
	// direct compact ID lookups without string-key reconstruction. Nil for
	// anomalies without a storage-backed series (e.g. log anomalies, RRCF).
	SourceRef *QueryHandle
	// DetectorName identifies which detector produced this anomaly.
	DetectorName string
	Title        string
	Description  string
	// Context carries optional enrichment about the originating signal, such as
	// a synthesized pattern and example source data.
	Context   *MetricContext
	Timestamp int64    // when the anomaly was detected (unix seconds)
	Score     *float64 // confidence/severity score (nil if not available)
	// SamplingIntervalSec is the median interval between consecutive data points
	// for the source series, in seconds. Set by scan detectors (ScanMW, ScanWelch)
	// at detection time from the actual point buffer. Zero if unknown.
	// Correlators use this to dynamically scale proximity windows so that
	// slow-sampling series (e.g. 15s redis check) can join clusters formed
	// by faster-sampling series (e.g. 10s trace stats).
	SamplingIntervalSec int64
	// DebugInfo contains detector-specific debug information explaining the detection.
	DebugInfo *AnomalyDebugInfo
}

// AnomalyDebugInfo provides detailed information about why an anomaly was detected.
type AnomalyDebugInfo struct {
	// Baseline statistics
	BaselineStart  int64   // timestamp of baseline period start
	BaselineEnd    int64   // timestamp of baseline period end
	BaselineMean   float64 // mean of baseline (for CUSUM)
	BaselineMedian float64 // median of baseline
	BaselineStddev float64 // stddev of baseline (for CUSUM)
	BaselineMAD    float64 // MAD of baseline

	// Detection parameters
	Threshold      float64 // threshold that was crossed
	SlackParam     float64 // k parameter (CUSUM only)
	CurrentValue   float64 // value at detection time
	DeviationSigma float64 // how many sigmas from baseline

	// For CUSUM: the cumulative sum values leading up to detection
	CUSUMValues []float64 // S[t] values (may be truncated to last N points)
}

// ReportOutput is the output model passed to reporters after each advance cycle.
// It carries enough data for reporters to act without reaching back into engine internals.
type ReportOutput struct {
	// AdvancedToSec is the data time the engine advanced to.
	AdvancedToSec int64
	// NewAnomalies are anomalies detected in this advance cycle.
	NewAnomalies []Anomaly
	// ActiveCorrelations are the current correlation patterns across all correlators.
	ActiveCorrelations []ActiveCorrelation
}

// Series is a time series with simple timestamp/value points.
// This is the simplified view passed to SeriesDetector.
type Series struct {
	Namespace string
	Name      string
	Tags      []string
	Points    []Point
}

// Point is a single timestamp/value pair.
type Point struct {
	Timestamp int64
	Value     float64
}

// MetricKind distinguishes gauge (absolute level) from counter (increment) telemetry.
// Gauge samples are exported with Set; counter samples with Add(value) on the backend counter.
type MetricKind int

const (
	// MetricKindGauge is the default: the metric value is an absolute level.
	MetricKindGauge MetricKind = iota
	// MetricKindCounter indicates the value is a delta added to the named counter.
	MetricKindCounter
)

// ObserverTelemetry describes a telemetry event emitted by the observer.
type ObserverTelemetry struct {
	DetectorName string
	Metric       MetricView
	Log          LogView
	// Kind is telemetry metric kind; zero means gauge (backward compatible).
	Kind MetricKind
}

// DetectionResult contains outputs from anomaly detection.
type DetectionResult struct {
	Anomalies []Anomaly
	// Used to debug anomaly detectors
	Telemetry []ObserverTelemetry
}

// SeriesDetector analyzes a time series for anomalies.
// Implementations should be stateless and just do math on the points.
type SeriesDetector interface {
	// Name returns the analysis name for debugging.
	Name() string
	// Detect examines a series and returns any detected anomalies.
	Detect(series Series) DetectionResult
}

// CorrelatorEventKind identifies the type of a correlator lifecycle event.
type CorrelatorEventKind int

const (
	// CorrelatorEventEpisodeStarted fires when the scorer enters High severity.
	CorrelatorEventEpisodeStarted CorrelatorEventKind = iota + 1
	// CorrelatorEventEpisodeEnded fires when the scorer leaves High severity.
	CorrelatorEventEpisodeEnded
	// CorrelatorEventCorrelationDetected fires when a correlator observes a
	// pattern for the first time (or after it has gone inactive and recurred).
	CorrelatorEventCorrelationDetected
)

// CorrelatorEvent is a typed lifecycle event produced by a correlator during Advance.
// Reporters receive these via ReportOutput.CorrelatorEvents and can emit backend
// notifications without relying on the one-shot dedup logic in ActiveCorrelations.
// Correlators own recurrence detection and produce CorrelationDetected events via
// a shared emitter; scorer-type correlators produce EpisodeStarted/EpisodeEnded.
type CorrelatorEvent struct {
	Kind CorrelatorEventKind
	// CorrelatorName identifies the correlator that produced this event.
	CorrelatorName string
	// Timestamp is the data time (unix seconds) when the event occurred.
	Timestamp int64
	// Correlation is the pattern associated with this event.
	// For EpisodeStarted: the newly opened episode (no end time yet).
	// For EpisodeEnded: the closed episode with the final LastUpdated.
	// For CorrelationDetected: the full active correlation at first-seen time.
	Correlation ActiveCorrelation
	// FromLevel and ToLevel carry the scorer severity transition.
	// Populated only for EpisodeStarted/EpisodeEnded; zero for CorrelationDetected.
	FromLevel severityeventsdef.SeverityLevel
	ToLevel   severityeventsdef.SeverityLevel
}

// Correlator accumulates anomaly events and produces correlated patterns.
// Correlators are stateful and cluster/filter/summarize anomaly streams.
//
// The lifecycle is: ProcessAnomaly to feed anomalies, Advance to trigger
// time-based maintenance (eviction, window finalization), and
// ActiveCorrelations to read current state.
type Correlator interface {
	// Name returns the correlator name for debugging.
	Name() string
	// ProcessAnomaly receives an anomaly event for accumulation.
	ProcessAnomaly(a Anomaly)
	// Advance performs time-based maintenance (eviction, window finalization)
	// up to the given data time. Callers should invoke this after each
	// detection cycle so correlators can manage their windows.
	Advance(dataTime int64)
	// ActiveCorrelations returns currently detected correlation patterns.
	ActiveCorrelations() []ActiveCorrelation
	// PendingEvents returns and drains typed lifecycle events accumulated during
	// the last Advance call. The caller owns the returned slice; the correlator
	// discards it after this call. Returns nil when no events are pending.
	// Correlators with no lifecycle events (e.g. time-cluster) always return nil.
	PendingEvents() []CorrelatorEvent
	// Reset clears all internal state for reanalysis.
	Reset()
}

// AnomalyScorerConfig holds the tunable parameters for the anomaly scoring pipeline.
type AnomalyScorerConfig struct {
	// Alpha is the EWMA smoothing factor (0 < α ≤ 1). Lower = smoother.
	Alpha float64 `json:"alpha"`
	// SaturationK is the saturation constant k: saturation = 1−exp(−n/k).
	// Calibrated against the window count (unique anomalous series), not per-second count.
	SaturationK float64 `json:"saturation_k"`
	// WindowSecs is the number of seconds a series stays in the active deduplication
	// window. A series seen at time t expires after t+WindowSecs. The saturation
	// function is applied to the number of unique series in the window, not to the
	// per-second event count.
	WindowSecs int64 `json:"window_secs"`
	// LowThreshold is the EWMA level defining the Low/Medium severity boundary.
	LowThreshold float64 `json:"low_threshold"`
	// HighThreshold is the EWMA level defining the Medium/High severity boundary.
	HighThreshold float64 `json:"high_threshold"`
	// MarginPct is the hysteresis margin as a fraction of HighThreshold.
	// effectiveMargin = HighThreshold × MarginPct.
	MarginPct float64 `json:"margin_pct"`
	// DetectorThresholds overrides the default score-to-level boundaries for
	// specific detector names. Each entry is [low, medium, high, xhigh] thresholds.
	// Detectors not in this map default to level 2 (Medium) regardless of their score.
	DetectorThresholds map[string][4]float64 `json:"detector_thresholds,omitempty"`
	// MaxBuckets overrides the number of AnomalyScoreBucket entries retained in
	// ScoreState(). 0 (default) means "cap at WindowSecs", which is the
	// correct behaviour for the live agent. Set to a large positive value
	// (e.g. math.MaxInt64) to keep an unlimited history for offline replay.
	MaxBuckets int64 `json:"max_buckets,omitempty"`
}

// AnomalyScoreBucket is the per-second telemetry unit emitted by the scorer.
// One bucket is produced for every 1-second tick, even if it has no anomalies.
type AnomalyScoreBucket struct {
	// Second is the Unix timestamp (floor) for this bucket.
	Second int64 `json:"second"`
	// Bins[L] is the number of deduplicated anomalies at level L (0=VeryLow … 4=XHigh).
	Bins [5]int `json:"bins"`
	// Count is the total number of anomalies in this bucket (sum of Bins).
	Count int `json:"count"`
	// WeightSum is the sum of level weights for all anomalies in this bucket.
	WeightSum float64 `json:"weight_sum"`
	// Ewma is the EWMA value after processing this bucket.
	Ewma float64 `json:"ewma"`
}

// AnomalyScoreState is the accumulated telemetry snapshot from the scorer.
type AnomalyScoreState struct {
	Buckets []AnomalyScoreBucket `json:"buckets"`
	Config  AnomalyScorerConfig  `json:"config"`
}

// Reporter receives reports and displays or delivers them.
type Reporter interface {
	// Name returns the reporter name for debugging.
	Name() string
	// Report delivers a report to its destination (stdout, file, webserver, etc).
	Report(report ReportOutput)
}

// ActiveCorrelation represents a detected correlation pattern.
type ActiveCorrelation struct {
	Pattern string // pattern name, e.g. "kernel_bottleneck"
	Title   string // display title, e.g. "Correlated: Kernel network bottleneck"
	// Members are the fully resolved series descriptors participating in this correlation.
	Members     []SeriesDescriptor
	Anomalies   []Anomaly // the actual anomalies that triggered this correlation
	FirstSeen   int64     // when pattern first matched (unix seconds, from data)
	LastUpdated int64     // most recent contributing signal (unix seconds, from data)
}

// RawAnomalyState provides read access to raw anomalies before correlation processing.
// Used by test bench reporters to display individual detector outputs.
type RawAnomalyState interface {
	// RawAnomalies returns all anomalies detected by detector implementations.
	RawAnomalies() []Anomaly
}

// TelemetryNamespace is the storage namespace used for observer-internal debug
// metrics (e.g. testbench UI charts). Detectors must not treat it as workload data.
const TelemetryNamespace = "telemetry"

// AgentNamespace is the storage namespace used for internal agent telemetry
// while normalizing datadog.* metrics before they are dropped.
const AgentNamespace = "agent"

// SeriesFilter specifies criteria for selecting series.
type SeriesFilter struct {
	Namespace   string            // exact match (empty = any)
	NamePattern string            // prefix match (empty = any)
	TagMatchers map[string]string // required tag key=value pairs
	// ExcludeNamespaces skips series whose namespace is in this list. It is only
	// applied when Namespace is empty (list-all mode). An explicit Namespace match
	// ignores ExcludeNamespaces so callers can still list internal series when needed.
	ExcludeNamespaces []string
}

// WorkloadSeriesFilter returns a filter for anomaly detectors: all namespaces
// except TelemetryNamespace.
func WorkloadSeriesFilter() SeriesFilter {
	return SeriesFilter{ExcludeNamespaces: []string{TelemetryNamespace}}
}

// SeriesMeta describes a series discovered via ListSeries.
// The Ref field is a stable numeric identifier for use in hot-path methods.
type SeriesMeta struct {
	Ref       SeriesRef
	Namespace string
	Name      string
	Tags      []string
}

// Aggregate specifies which statistic to extract from summary stats.
type Aggregate int

const (
	AggregateNone Aggregate = iota
	AggregateAverage
	AggregateSum
	AggregateCount
	AggregateMin
	AggregateMax
)

// AggregateString returns a short string label for the aggregation type.
func AggregateString(agg Aggregate) string {
	switch agg {
	case AggregateNone:
		return "none"
	case AggregateAverage:
		return "avg"
	case AggregateSum:
		return "sum"
	case AggregateCount:
		return "count"
	case AggregateMin:
		return "min"
	case AggregateMax:
		return "max"
	default:
		return "unknown"
	}
}

// MetricContext describes the origin of a synthesized metric.
type MetricContext struct {
	// Pattern is the normalized pattern that generated this metric (e.g. a log signature).
	Pattern string
	// Example is a recent raw input that matched the pattern.
	Example string
	// Source identifies the originating component or data stream.
	Source string
	// SplitTags carries the tag-group key/value pairs (source, service, env, host) that
	// scoped the sub-clusterer which produced this metric. Nil when no split tags apply.
	SplitTags map[string]string
}

// StorageReader provides read access to time series data.
// Detectors use this to pull whatever data they need.
//
// Use ListSeries to discover series and obtain their numeric handles.
// All hot-path methods take a SeriesRef for O(1) lookups.
//
// Reading points: ForEachPoint and GetSeriesRange both read the same data;
// they differ in allocation cost and ownership model.
//
//   - ForEachPoint reuses a pooled buffer internally — effectively zero
//     allocation at steady state. The callback sees each point exactly once
//     and must not retain the *Series pointer. Prefer this for streaming or
//     incremental callers that process points one at a time.
//
//   - GetSeriesRange allocates a fresh []Point each call. The caller owns the
//     returned data and may slice, index, or store it freely. Prefer this when
//     the detector needs random access to the full window (e.g. baseline
//     estimation, cross-series alignment).
//
// Use PointCountUpTo and WriteGeneration to cheaply detect new data before
// reading points.
type StorageReader interface {
	// ListSeries returns metadata for all series matching the filter.
	ListSeries(filter SeriesFilter) []SeriesMeta

	// GetSeriesRange returns points within a time range (start, end].
	// Start is exclusive, end is inclusive. Use start=0 to read from the beginning.
	// Allocates a new []Point slice — see interface doc for when to prefer ForEachPoint.
	GetSeriesRange(handle SeriesRef, start, end int64, agg Aggregate) *Series

	// ForEachPoint calls fn for every point in the time range (start, end].
	// The Series pointer and its contents are valid only for the duration of
	// the callback. Uses a pooled buffer internally so steady-state calls
	// do not allocate. Returns false if the series was not found.
	ForEachPoint(handle SeriesRef, start, end int64, agg Aggregate, fn func(*Series, Point)) bool

	// PointCount returns the number of raw data points for a series without
	// loading or converting them. Returns 0 if the series is not found.
	PointCount(handle SeriesRef) int

	// PointCountUpTo returns the number of raw data points with timestamp <= endTime.
	// Uses binary search for efficiency. Returns 0 if the series is not found.
	PointCountUpTo(handle SeriesRef, endTime int64) int

	// SumRange returns the sum of the specified aggregate over all points with
	// timestamp in (start, end] without allocating any intermediate slices.
	// Returns 0 if the series is not found or the range is empty.
	// This is more efficient than ForEachPoint when only the aggregate total
	// is needed (e.g. computing an average rate over a window).
	SumRange(handle SeriesRef, start, end int64, agg Aggregate) float64

	// WriteGeneration returns a per-series counter that increments on every
	// write to that series, including same-bucket merges. Use this to detect
	// updates to an existing series even when its point count does not change.
	// Returns 0 if the series is not found.
	WriteGeneration(handle SeriesRef) int64

	// SeriesGeneration returns a global counter that increments only when the
	// set of known series changes. Use this to cache ListSeries results and
	// refresh them only when new series keys appear.
	SeriesGeneration() uint64
}

// Detector is the flexible detection interface where detectors pull data from storage.
// This supports multivariate detection across multiple series.
type Detector interface {
	Name() string

	// Detect is called periodically by the scheduler.
	// The detector queries storage for whatever data it needs.
	// dataTime is the current data timestamp (for determinism - only read data <= dataTime).
	Detect(storage StorageReader, dataTime int64) DetectionResult
}

// SeriesRemover is an optional interface that Detector implementations can
// satisfy to receive notifications when storage drops series.
//
// Many detectors maintain per-series state (BOCPD posterior arrays, ScanMW
// segment buffers, ScanWelch posterior, the seriesDetectorAdapter visible
// point count map, etc.) keyed by SeriesRef. Storage frees the series
// payload itself when extractors evict their LRU contexts and the engine
// calls RemoveSeriesByKeys, but without this hook the detector-side maps
// keep growing unbounded with the cumulative number of series ever
// observed. The engine fans the freed refs out to every detector that
// implements this interface immediately after RemoveSeriesByKeys returns
// them, keeping detector state symmetric with storage state.
//
// Implementations should be cheap (a handful of map deletes) and tolerant
// of refs they have never seen — adapters routinely receive refs for
// series they were never asked to detect on (e.g. metric series on a
// log-only detector).
type SeriesRemover interface {
	RemoveSeries(refs []SeriesRef)
}
