// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hook

// MetricView provides read-only access to a metric sample.
// It is used internally as the input type for NewMetricSampleSnapshot.
type MetricView interface {
	GetName() string
	GetValue() float64
	GetRawTags() []string
	GetTimestamp() float64
	GetSampleRate() float64
}

// MetricSampleSnapshot is an immutable, pool-safe snapshot of a metric sample.
// Unlike the original MetricSample (which is pooled and recycled), a snapshot
// holds copied values and is safe to retain across goroutine boundaries.
//
// ContextKey is the aggregator's context key (murmur3 hash of name+hostname+tags).
// Subscribers can use it as a precomputed deduplication key, avoiding a redundant
// hash computation. It is 0 for pipelines that do not run a context resolver
// (no-aggregation, check sampler).
type MetricSampleSnapshot struct {
	Name       string
	Value      float64
	RawTags    []string
	Timestamp  float64
	SampleRate float64
	ContextKey uint64
}

// NewMetricSampleSnapshot copies the observable fields from v into a new snapshot.
func NewMetricSampleSnapshot(v MetricView) MetricSampleSnapshot {
	return MetricSampleSnapshot{
		Name:       v.GetName(),
		Value:      v.GetValue(),
		RawTags:    v.GetRawTags(),
		Timestamp:  v.GetTimestamp(),
		SampleRate: v.GetSampleRate(),
	}
}

// LogView provides read-only access to a log message.
type LogView interface {
	GetContent() []byte
	GetStatus() string
	GetTags() []string
	GetHostname() string
}

// TraceStatsView provides read-only access to an aggregated trace stats entry.
type TraceStatsView interface {
	// Grouped stats fields (from ClientGroupedStats).
	GetService() string
	GetName() string
	GetResource() string
	GetType() string
	GetHTTPStatusCode() uint32
	GetSpanKind() string
	GetHits() uint64
	GetErrors() uint64
	GetDuration() uint64
	GetTopLevelHits() uint64
	GetOkSummary() []byte
	GetErrorSummary() []byte

	// Envelope fields from ClientStatsPayload / ClientStatsBucket.
	GetHostname() string
	GetEnv() string
	GetVersion() string
	GetBucketStartNs() uint64
	GetBucketDurationNs() uint64
}
