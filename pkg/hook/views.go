// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hook

// MetricView provides read-only access to a metric sample.
type MetricView interface {
	GetName() string
	GetValue() float64
	GetRawTags() []string
	GetTimestamp() float64
	GetSampleRate() float64
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
