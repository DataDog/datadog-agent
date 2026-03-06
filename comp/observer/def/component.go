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
// Implementations should be stateless and fast since they run synchronously
// on every observed log.
type LogMetricsExtractor interface {
	// Name returns the extractor name for debugging and logging.
	Name() string
	// ProcessLog examines a log and returns any derived metrics.
	ProcessLog(log LogView) []MetricOutput
}

// LogObserver is an optional interface that Detectors can implement to
// receive log observations. This allows detectors to incorporate log signals
// without going through the metrics extraction path.
type LogObserver interface {
	ProcessLog(log LogView)
}

// MetricOutput is a timeseries value derived from log analysis.
// The storage keeps full summaries (min/max/sum/count) so aggregation
// is specified at read time, not write time.
type MetricOutput struct {
	Name  string
	Value float64
	Tags  []string
}

// MetricName is a human-readable metric identifier (e.g., "cpu.user:avg").
// Multiple series can share a MetricName if they differ by tags.
type MetricName string

// SeriesID uniquely identifies a time series (namespace + name + tags).
type SeriesID string

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
	// Source identifies which metric/signal or log source the anomaly is about.
	// For metric anomalies: the metric name (e.g., "network.retransmits:avg").
	// For log anomalies: a descriptive source identifier (e.g., "logs").
	Source MetricName
	// SourceSeriesID uniquely identifies the source series (namespace + name + tags).
	// Empty for log anomalies.
	SourceSeriesID SeriesID
	// DetectorName identifies which detector produced this anomaly.
	DetectorName string
	Title        string
	Description  string
	Tags         []string
	Timestamp    int64    // when the anomaly was detected (unix seconds)
	Score        *float64 // confidence/severity score (nil if not available)
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

// ReportOutput is a processed summary from correlators.
// It represents clustered, filtered, or summarized anomaly information ready for display.
type ReportOutput struct {
	Title    string
	Body     string
	Metadata map[string]string
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

// Describes a telemetry event that is emitted by the observer.
// This could be a metric or a log for instance.
type ObserverTelemetry struct {
	DetectorName string
	Metric       MetricView
	Log          LogView
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

// Correlator accumulates anomaly events and produces reports.
// Correlators are stateful and cluster/filter/summarize anomaly streams.
type Correlator interface {
	// Name returns the correlator name for debugging.
	Name() string
	// Process receives an anomaly event for accumulation.
	Process(anomaly Anomaly)
	// Flush processes accumulated anomalies and returns reports.
	Flush() []ReportOutput
}

// Reporter receives reports and displays or delivers them.
type Reporter interface {
	// Name returns the reporter name for debugging.
	Name() string
	// Report delivers a report to its destination (stdout, file, webserver, etc).
	Report(report ReportOutput)
}

// CorrelationState provides read access to active correlations.
// Reporters use this to display current correlation status.
type CorrelationState interface {
	// ActiveCorrelations returns currently detected correlation patterns.
	ActiveCorrelations() []ActiveCorrelation
}

// ActiveCorrelation represents a detected correlation pattern.
type ActiveCorrelation struct {
	Pattern string // pattern name, e.g. "kernel_bottleneck"
	Title   string // display title, e.g. "Correlated: Kernel network bottleneck"
	// MemberSeriesIDs are the concrete series identities participating in this correlation.
	MemberSeriesIDs []SeriesID
	// MetricNames are display-oriented metric names participating in this correlation.
	MetricNames []MetricName
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

// SeriesFilter specifies criteria for selecting series.
type SeriesFilter struct {
	Namespace   string            // exact match (empty = any)
	NamePattern string            // prefix match (empty = any)
	TagMatchers map[string]string // required tag key=value pairs
}

// SeriesKey identifies a specific series.
type SeriesKey struct {
	Namespace string
	Name      string
	Tags      []string
}

// Aggregate specifies which statistic to extract from summary stats.
type Aggregate int

const (
	AggregateAverage Aggregate = iota
	AggregateSum
	AggregateCount
	AggregateMin
	AggregateMax
)

// StorageReader provides read access to time series data.
// Detectors use this to pull whatever data they need.
type StorageReader interface {
	// ListSeries returns keys of all series matching the filter.
	ListSeries(filter SeriesFilter) []SeriesKey

	// GetSeriesRange returns points within a time range (start, end].
	// Start is exclusive, end is inclusive. Use start=0 to read from the beginning.
	GetSeriesRange(key SeriesKey, start, end int64, agg Aggregate) *Series

	// PointCount returns the number of raw data points for a series without
	// loading or converting them. Returns 0 if the series is not found.
	PointCount(key SeriesKey) int
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
