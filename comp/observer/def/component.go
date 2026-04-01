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
)

// team: agent-metric-pipelines

// Component is the central observer that receives data via handles.
type Component interface {
	// GetHandle returns a lightweight handle for a named source.
	// The source name is used to identify where observations originate.
	GetHandle(name string) Handle

	// DumpMetrics writes all stored metrics to the specified file (for debugging).
	DumpMetrics(path string) error
}

// Handle is the lightweight observation interface passed to other components.
type Handle interface {
	// ObserveMetric observes a DogStatsD metric sample.
	ObserveMetric(sample MetricView)

	// ObserveLog observes a log message.
	ObserveLog(msg LogView)

	// ObserveTrace observes a trace (collection of spans with the same trace ID).
	ObserveTrace(trace TraceView)

	// ObserveTraceStats observes an APM stats payload (aggregated counts and
	// latency distributions computed by the trace concentrator).
	ObserveTraceStats(stats TraceStatsView)

	// ObserveProfile observes a profiling sample.
	ObserveProfile(profile ProfileView)
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
	GetContent() []byte
	GetStatus() string
	GetTags() []string
	GetHostname() string
	// GetTimestampUnixMilli returns the log timestamp in Unix milliseconds.
	GetTimestampUnixMilli() int64
}

// TraceView provides read-only access to a trace (collection of spans with the same trace ID).
//
// This interface exists to prevent data races. The underlying trace data may be
// reused immediately after ObserveTrace returns, so implementations must not
// store the TraceView itself. Copy any needed values synchronously.
type TraceView interface {
	// GetTraceID returns the trace ID as high and low 64-bit parts.
	GetTraceID() (high, low uint64)
	// GetSpans returns an iterator over spans in this trace.
	GetSpans() SpanIterator
	// GetEnv returns the environment tag for this trace.
	GetEnv() string
	// GetService returns the primary service name for this trace.
	GetService() string
	// GetHostname returns the hostname where the trace originated.
	GetHostname() string
	// GetContainerID returns the container ID where the trace originated.
	GetContainerID() string
	// GetTimestampUnixNano returns the trace start time in Unix nanoseconds.
	GetTimestampUnixNano() int64
	// GetDurationNano returns the trace duration in nanoseconds.
	GetDurationNano() int64
	// GetPriority returns the sampling priority.
	GetPriority() int32
	// IsError returns true if this trace contains an error.
	IsError() bool
	// GetTags returns trace-level tags.
	GetTags() map[string]string
}

// SpanIterator provides iteration over spans in a trace.
// It follows the Go iterator pattern where Next() advances and returns
// whether there are more spans, and Span() returns the current span.
type SpanIterator interface {
	// Next advances to the next span and returns true if there is one.
	Next() bool
	// Span returns the current span. Only valid after Next() returns true.
	Span() SpanView
	// Reset resets the iterator to the beginning.
	Reset()
}

// SpanView provides read-only access to a single span within a trace.
type SpanView interface {
	// GetSpanID returns the unique span identifier.
	GetSpanID() uint64
	// GetParentID returns the parent span ID (0 for root spans).
	GetParentID() uint64
	// GetService returns the service name for this span.
	GetService() string
	// GetName returns the operation name (span name).
	GetName() string
	// GetResource returns the resource name (e.g., SQL query, HTTP route).
	GetResource() string
	// GetType returns the span type (e.g., "web", "db", "cache").
	GetType() string
	// GetStartUnixNano returns the span start time in Unix nanoseconds.
	GetStartUnixNano() int64
	// GetDurationNano returns the span duration in nanoseconds.
	GetDurationNano() int64
	// GetError returns the error code (0 = no error, 1 = error).
	GetError() int32
	// GetMeta returns string tags/metadata for this span.
	GetMeta() map[string]string
	// GetMetrics returns numeric metrics for this span.
	GetMetrics() map[string]float64
}

// TraceStatsView provides read-only access to an APM stats payload.
// Stats represent aggregated counts and latency distributions per (service, resource, operation)
// group over a time bucket. The view iterates over denormalized rows combining payload-,
// client-, bucket-, and group-level fields.
//
// This interface exists to prevent data races. Implementations must not store the view.
// Copy any needed values synchronously before ObserveTraceStats returns.
type TraceStatsView interface {
	// GetAgentHostname returns the agent hostname that processed these stats.
	GetAgentHostname() string
	// GetAgentEnv returns the agent environment.
	GetAgentEnv() string
	// GetRows returns an iterator over denormalized stat rows.
	// Each row combines payload, client, bucket, and grouped-stats fields.
	GetRows() TraceStatsRowIterator
}

// TraceStatsRowIterator iterates over denormalized rows of a TraceStatsView.
type TraceStatsRowIterator interface {
	// Next advances to the next row. Returns false when exhausted.
	Next() bool
	// Row returns the current row. Only valid after Next() returns true.
	Row() TraceStatRow
}

// TraceStatRow represents one aggregated stat group with its context.
// It combines fields from ClientStatsPayload, ClientStatsBucket, and ClientGroupedStats.
type TraceStatRow interface {
	// Client-level context (from ClientStatsPayload)
	GetClientHostname() string
	GetClientEnv() string
	GetClientVersion() string
	GetClientContainerID() string
	// Time bucket window (from ClientStatsBucket)
	GetBucketStartUnixNano() uint64 // Unix nanoseconds
	GetBucketDurationNano() uint64  // nanoseconds
	// Aggregation dimensions (from ClientGroupedStats)
	GetService() string
	GetName() string // operation name
	GetResource() string
	GetType() string
	GetHTTPStatusCode() uint32
	GetSpanKind() string
	GetIsTraceRoot() int32 // 0=NOT_SET, 1=TRUE, 2=FALSE
	GetSynthetics() bool
	// Aggregated values (from ClientGroupedStats)
	GetHits() uint64
	GetErrors() uint64
	GetTopLevelHits() uint64
	GetDurationNano() uint64 // total duration in nanoseconds
	GetOkSummary() []byte    // DDSketch encoded latency distribution for ok spans
	GetErrorSummary() []byte // DDSketch encoded latency distribution for error spans
	GetPeerTags() []string
}

// ProfileView provides read-only access to a profiling sample.
//
// This interface exists to prevent data races. Implementations must not store
// the ProfileView itself. Copy any needed values synchronously.
// Note: Profile format is language-agnostic (pprof for Go/Python, JFR for Java, etc.).
// The observer stores profiles as opaque binary blobs without parsing or transforming them.
type ProfileView interface {
	// GetProfileID returns a unique identifier for this profile.
	GetProfileID() string
	// GetProfileType returns the profile type (cpu, heap, mutex, etc.).
	GetProfileType() string
	// GetService returns the service name that produced this profile.
	GetService() string
	// GetEnv returns the environment tag.
	GetEnv() string
	// GetVersion returns the application version.
	GetVersion() string
	// GetHostname returns the hostname where the profile was collected.
	GetHostname() string
	// GetContainerID returns the container ID where the profile was collected.
	GetContainerID() string
	// GetTimestampUnixNano returns the profile timestamp in Unix nanoseconds.
	GetTimestampUnixNano() int64
	// GetDurationNano returns the profile duration in nanoseconds.
	GetDurationNano() int64
	// GetTags returns profile tags.
	GetTags() map[string]string
	// GetContentType returns the original Content-Type header (profile format is language-specific).
	GetContentType() string
	// GetRawData returns the opaque binary profile data (nil if stored externally).
	GetRawData() []byte
	// GetExternalPath returns the path to an external binary file (empty if inline).
	GetExternalPath() string
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
	Name       string
	Value      float64
	Tags       []string
	ContextKey string
}

// LogMetricsExtractorOutput is what we obtain when we process a log with a log metrics extractor.
type LogMetricsExtractorOutput struct {
	Metrics   []MetricOutput
	Telemetry []ObserverTelemetry
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

// Describes a telemetry event that is emitted by the observer.
// This could be a metric or a log for instance.
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
	// Reset clears all internal state for reanalysis.
	Reset()
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

// ContextProvider resolves metric keys back to richer context about their
// origin. Components that synthesize metrics from richer data (e.g. log
// extractors that turn log patterns into count metrics) can implement this
// interface so that downstream consumers (detectors, reporters) can produce
// more descriptive anomaly reports.
type ContextProvider interface {
	// GetContextByKey returns contextual information for a previously emitted
	// context key, if available.
	GetContextByKey(key string) (MetricContext, bool)
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
