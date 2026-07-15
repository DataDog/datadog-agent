// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build openmetrics_differential

// Package differential is a throwaway differential-testing harness that runs the
// Go OpenMetrics check and the Python OpenMetrics check against identical inputs
// (via an httptest.Server both implementations point at) and diffs the recorded
// submissions. Gated behind the openmetrics_differential build tag so it never
// runs in normal CI.
package differential

import (
	"sort"

	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"
	"github.com/DataDog/datadog-agent/pkg/util/infratags"
)

// Submission is the wire-format shared with the Python sidecar. The schema is
// intentionally narrow: kind, name, value, tags (canonical-sorted), hostname.
// Anything that doesn't survive a round-trip through JSON (function pointers,
// goroutines, et al.) stays out of the schema.
//
// FlushFirstValue is only meaningful for monotonic_count submissions. It
// records whether the scraper considered this the "first" observation of the
// counter — i.e., whether downstream rate math should treat it as a seed
// value rather than a real delta. Stateful (two-scrape) tests use this to
// verify the flag toggles correctly across consecutive scrapes; stateless
// tests can ignore it (it's omitted from JSON when false).
type Submission struct {
	Kind            string   `json:"kind"`
	Name            string   `json:"name"`
	Value           float64  `json:"value"`
	Tags            []string `json:"tags"`
	Hostname        string   `json:"hostname"`
	Message         string   `json:"message,omitempty"`
	FlushFirstValue bool     `json:"flush_first_value,omitempty"`
}

// RecordingSender implements sender.Sender by appending every submission to a
// slice. It is single-threaded — the OpenMetrics scraper does not call the
// sender concurrently.
type RecordingSender struct {
	Submissions []Submission
}

func (r *RecordingSender) append(kind, name string, value float64, hostname string, tags []string) {
	sorted := make([]string, len(tags))
	copy(sorted, tags)
	sort.Strings(sorted)
	r.Submissions = append(r.Submissions, Submission{
		Kind: kind, Name: name, Value: value, Hostname: hostname, Tags: sorted,
	})
}

func (r *RecordingSender) Commit() {}

func (r *RecordingSender) Gauge(m string, v float64, h string, t []string) {
	r.append("gauge", m, v, h, t)
}
func (r *RecordingSender) GaugeNoIndex(m string, v float64, h string, t []string) {
	r.append("gauge", m, v, h, t)
}
func (r *RecordingSender) Rate(m string, v float64, h string, t []string) {
	r.append("rate", m, v, h, t)
}
func (r *RecordingSender) Count(m string, v float64, h string, t []string) {
	r.append("count", m, v, h, t)
}
func (r *RecordingSender) MonotonicCount(m string, v float64, h string, t []string) {
	r.append("monotonic_count", m, v, h, t)
}
func (r *RecordingSender) MonotonicCountWithFlushFirstValue(m string, v float64, h string, t []string, flushFirstValue bool) {
	sorted := make([]string, len(t))
	copy(sorted, t)
	sort.Strings(sorted)
	r.Submissions = append(r.Submissions, Submission{
		Kind: "monotonic_count", Name: m, Value: v, Hostname: h, Tags: sorted,
		FlushFirstValue: flushFirstValue,
	})
}
func (r *RecordingSender) Counter(m string, v float64, h string, t []string) {
	r.append("count", m, v, h, t)
}
func (r *RecordingSender) Histogram(m string, v float64, h string, t []string) {
	r.append("histogram", m, v, h, t)
}
func (r *RecordingSender) Historate(m string, v float64, h string, t []string) {
	r.append("historate", m, v, h, t)
}
func (r *RecordingSender) Distribution(m string, v float64, h string, t []string) {
	r.append("histogram", m, v, h, t) // Python's `distribution` maps to histogram-as-distribution; treat as histogram for diff purposes
}
func (r *RecordingSender) ServiceCheck(name string, status servicecheck.ServiceCheckStatus, h string, tags []string, message string) {
	sorted := make([]string, len(tags))
	copy(sorted, tags)
	sort.Strings(sorted)
	r.Submissions = append(r.Submissions, Submission{
		Kind: "service_check", Name: name, Value: float64(status), Hostname: h, Tags: sorted, Message: message,
	})
}

// HistogramBucket / OpenmetricsBucket: not used by the Go OpenMetrics scrape
// path's typical config, but recorded so we don't drop signals on the floor.
func (r *RecordingSender) HistogramBucket(m string, value int64, _, _ float64, _ bool, h string, t []string, _ bool) {
	r.append("histogram_bucket", m, float64(value), h, t)
}
func (r *RecordingSender) OpenmetricsBucket(m string, value int64, _, _ float64, _ bool, h string, t []string, _ bool) {
	r.append("openmetrics_bucket", m, float64(value), h, t)
}

func (r *RecordingSender) GaugeWithTimestamp(m string, v float64, h string, t []string, _ float64) error {
	r.append("gauge", m, v, h, t)
	return nil
}
func (r *RecordingSender) CountWithTimestamp(m string, v float64, h string, t []string, _ float64) error {
	r.append("count", m, v, h, t)
	return nil
}

// The rest of the Sender interface are no-ops for differential-testing purposes.
func (r *RecordingSender) Event(event.Event)                                            {}
func (r *RecordingSender) EventPlatformEvent([]byte, string)                            {}
func (r *RecordingSender) GetSenderStats() stats.SenderStats                            { return stats.SenderStats{} }
func (r *RecordingSender) DisableDefaultHostname(bool)                                  {}
func (r *RecordingSender) SetCheckCustomTags([]string)                                  {}
func (r *RecordingSender) SetCheckService(string)                                       {}
func (r *RecordingSender) SetNoIndex(bool)                                              {}
func (r *RecordingSender) SetInfraTagger(*infratags.Tagger)                             {}
func (r *RecordingSender) FinalizeCheckServiceTag()                                     {}
func (r *RecordingSender) OrchestratorMetadata([]types.ProcessMessageBody, string, int) {}
func (r *RecordingSender) OrchestratorManifest([]types.ProcessMessageBody, string)      {}
