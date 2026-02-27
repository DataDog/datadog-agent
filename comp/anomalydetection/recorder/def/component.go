// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package recorder provides a middleware component for recording and replaying observer data.
//
// The recorder intercepts metrics flowing through observer handles, optionally
// recording them to parquet files. This enables:
// - Capturing production data for offline analysis
// - Loading recorded data for testing and debugging
// - Building reproducible test scenarios from real workloads
package recorder

import (
	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// team: agent-metric-pipelines

// Component is the recorder middleware component.
// It wraps observer handles to intercept and optionally record observations.
type Component interface {
	// GetHandle wraps the provided HandleFunc with recording capability.
	// If recording is enabled via config, metrics will be written to parquet files.
	// This is called by the observer's GetHandle to create the final handle chain.
	GetHandle(handleFunc observer.HandleFunc) observer.HandleFunc

	// ReadAllMetrics reads all metrics from parquet files and returns them as a slice.
	// This is for batch loading scenarios (like testbench) where streaming via handles
	// is not needed and direct access to all metrics at once is more efficient.
	ReadAllMetrics(inputDir string) ([]MetricData, error)

	// ReadAllTraces reads all traces from parquet files and returns them as a slice.
	// Traces are stored as denormalized spans (one row per span) for efficient querying.
	ReadAllTraces(inputDir string) ([]TraceData, error)

	// ReadAllProfiles reads all profile metadata from parquet files and returns them as a slice.
	// Profile binary data is stored in separate files referenced by BinaryPath.
	ReadAllProfiles(inputDir string) ([]ProfileData, error)

	// ReadAllLogs reads all logs from parquet files and returns them as a slice.
	ReadAllLogs(inputDir string) ([]LogData, error)

	// ReadAllTraceStats reads all APM trace stats from parquet files and returns them as a slice.
	// Each element corresponds to one aggregated stat group (ClientGroupedStats).
	ReadAllTraceStats(inputDir string) ([]TraceStatsData, error)
}

// MetricData represents a single metric read from parquet files.
// Used by ReadAllMetrics for batch loading scenarios.
type MetricData struct {
	Source    string   // Source/namespace (RunID in parquet)
	Name      string   // Metric name
	Value     float64  // Metric value
	Timestamp int64    // Unix timestamp in seconds
	Tags      []string // Tags in "key:value" format
}

// TraceData represents a trace read from parquet files.
// Traces are reconstructed from denormalized span rows grouped by trace ID.
type TraceData struct {
	Source      string     // Source/namespace (RunID in parquet)
	TraceIDHigh uint64     // High 64 bits of trace ID
	TraceIDLow  uint64     // Low 64 bits of trace ID
	Env         string     // Environment tag
	Service     string     // Primary service name
	Hostname    string     // Hostname where trace originated
	ContainerID string     // Container ID where trace originated
	Timestamp   int64      // Trace start time in nanoseconds since epoch
	Duration    int64      // Trace duration in nanoseconds
	Priority    int32      // Sampling priority
	IsError     bool       // Whether trace contains an error
	Tags        []string   // Trace-level tags in "key:value" format
	Spans       []SpanData // All spans in this trace
}

// SpanData represents a single span within a trace.
type SpanData struct {
	SpanID   uint64   // Unique span identifier
	ParentID uint64   // Parent span ID (0 for root spans)
	Service  string   // Service name for this span
	Name     string   // Operation name (span name)
	Resource string   // Resource name (e.g., SQL query, HTTP route)
	Type     string   // Span type (e.g., "web", "db", "cache")
	Start    int64    // Span start time in nanoseconds since epoch
	Duration int64    // Span duration in nanoseconds
	Error    int32    // Error code (0 = no error, 1 = error)
	Meta     []string // String tags in "key:value" format
	Metrics  []string // Numeric metrics in "key:value" format (value as string)
}

// ProfileData represents a profile read from parquet files.
//
// Note: The trace-agent acts purely as a reverse proxy for profiles - it doesn't
// parse the profile binary data at all. Profiles are opaque binary blobs whose
// format is language-specific (pprof for Go/Python, JFR for Java, etc.). The
// trace-agent simply forwards the HTTP request to the Datadog backend, adding
// headers like Via, DD-API-KEY, X-Datadog-Additional-Tags, and X-Datadog-Container-Tags.
//
// This means we store profiles as opaque binaries with metadata extracted from
// the HTTP headers and form fields, not from parsing the profile content itself.
// The binary data is embedded directly in the parquet file for simplicity.
type ProfileData struct {
	Source      string   // Source/namespace (RunID in parquet)
	ProfileID   string   // Unique profile identifier
	ProfileType string   // Profile type (cpu, heap, mutex, etc.)
	Service     string   // Service name
	Env         string   // Environment tag
	Version     string   // Application version
	Hostname    string   // Hostname where profile was collected
	ContainerID string   // Container ID where profile was collected
	Timestamp   int64    // Profile timestamp in nanoseconds since epoch
	Duration    int64    // Profile duration in nanoseconds
	ContentType string   // Original Content-Type header
	BinaryData  []byte   // Embedded profile binary (pprof, JFR, etc.)
	Tags        []string // Profile tags in "key:value" format
}

// TraceStatsData represents one aggregated stat group read from parquet.
// It corresponds to one ClientGroupedStats entry with its payload/client/bucket context.
type TraceStatsData struct {
	Source            string   // Source/namespace (RunID in parquet)
	AgentHostname     string   // Agent hostname that processed these stats
	AgentEnv          string   // Agent environment
	ClientHostname    string   // Tracer hostname
	ClientEnv         string   // Tracer environment
	ClientVersion     string   // Application version
	ClientContainerID string   // Container ID
	BucketStart       uint64   // Bucket start time in nanoseconds since epoch
	BucketDuration    uint64   // Bucket duration in nanoseconds
	Service           string   // Service name (aggregation dimension)
	Name              string   // Operation name (aggregation dimension)
	Resource          string   // Resource name (aggregation dimension)
	Type              string   // Span type (aggregation dimension)
	HTTPStatusCode    uint32   // HTTP status code (aggregation dimension)
	SpanKind          string   // Span kind (aggregation dimension)
	IsTraceRoot       int32    // 0=NOT_SET, 1=TRUE, 2=FALSE (aggregation dimension)
	Synthetics        bool     // Whether this is a synthetic trace
	Hits              uint64   // Total request count
	Errors            uint64   // Error count
	TopLevelHits      uint64   // Top-level span count
	Duration          uint64   // Total duration in nanoseconds
	OkSummary         []byte   // DDSketch encoded latency distribution for ok spans
	ErrorSummary      []byte   // DDSketch encoded latency distribution for error spans
	PeerTags          []string // Peer entity tags (e.g., "db.hostname:...")
}

// LogData represents a log entry read from parquet files.
type LogData struct {
	Source    string   // Source/namespace (RunID in parquet)
	Timestamp int64    // Unix timestamp in milliseconds since epoch
	Content   []byte   // Log message content (raw bytes)
	Status    string   // Log severity level (debug, info, warn, error, etc.)
	Hostname  string   // Hostname where log originated
	Tags      []string // Tags in "key:value" format
}
