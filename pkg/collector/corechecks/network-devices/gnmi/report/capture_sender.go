// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package report

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"
	"github.com/DataDog/datadog-agent/pkg/util/infratags"
)

var _ sender.Sender = (*CaptureSender)(nil)

// CapturedMetric records a metric submission intercepted by CaptureSender.
type CapturedMetric struct {
	Type     string
	Name     string
	Value    float64
	Hostname string
	Tags     []string
}

// CapturedMetadataEvent records an event platform submission intercepted by CaptureSender.
type CapturedMetadataEvent struct {
	EventType string
	Payload   []byte
}

// CaptureSender implements sender.Sender and records metric submissions.
type CaptureSender struct {
	metrics  []CapturedMetric
	metadata []CapturedMetadataEvent
}

// NewCaptureSender creates a sender that records submitted metrics.
func NewCaptureSender() *CaptureSender {
	return &CaptureSender{}
}

// Metrics returns captured metrics in submission order.
func (c *CaptureSender) Metrics() []CapturedMetric {
	if c == nil {
		return nil
	}
	return c.metrics
}

// MetadataEvents returns captured metadata event platform payloads in submission order.
func (c *CaptureSender) MetadataEvents() []CapturedMetadataEvent {
	if c == nil {
		return nil
	}
	return c.metadata
}

// Reset clears captured metrics and metadata events.
func (c *CaptureSender) Reset() {
	if c == nil {
		return
	}
	c.metrics = nil
	c.metadata = nil
}

func (c *CaptureSender) record(metricType string, name string, value float64, hostname string, tags []string) {
	c.metrics = append(c.metrics, CapturedMetric{
		Type:     metricType,
		Name:     name,
		Value:    value,
		Hostname: hostname,
		Tags:     append([]string(nil), tags...),
	})
}

func (c *CaptureSender) Commit() {}

func (c *CaptureSender) Gauge(metric string, value float64, hostname string, tags []string) {
	c.record("gauge", metric, value, hostname, tags)
}

func (c *CaptureSender) GaugeNoIndex(metric string, value float64, hostname string, tags []string) {
	c.record("gauge_no_index", metric, value, hostname, tags)
}

func (c *CaptureSender) Rate(metric string, value float64, hostname string, tags []string) {
	c.record("rate", metric, value, hostname, tags)
}

func (c *CaptureSender) Count(metric string, value float64, hostname string, tags []string) {
	c.record("count", metric, value, hostname, tags)
}

func (c *CaptureSender) MonotonicCount(metric string, value float64, hostname string, tags []string) {
	c.record("monotonic_count", metric, value, hostname, tags)
}

func (c *CaptureSender) MonotonicCountWithFlushFirstValue(metric string, value float64, hostname string, tags []string, _ bool) {
	c.record("monotonic_count", metric, value, hostname, tags)
}

func (c *CaptureSender) Counter(metric string, value float64, hostname string, tags []string) {
	c.record("counter", metric, value, hostname, tags)
}

func (c *CaptureSender) Histogram(metric string, value float64, hostname string, tags []string) {
	c.record("histogram", metric, value, hostname, tags)
}

func (c *CaptureSender) Historate(metric string, value float64, hostname string, tags []string) {
	c.record("historate", metric, value, hostname, tags)
}

func (c *CaptureSender) Distribution(metric string, value float64, hostname string, tags []string) {
	c.record("distribution", metric, value, hostname, tags)
}

func (c *CaptureSender) ServiceCheck(checkName string, status servicecheck.ServiceCheckStatus, hostname string, tags []string, _ string) {
	c.record("service_check", checkName, float64(status), hostname, tags)
}

func (c *CaptureSender) OpenmetricsBucket(metric string, value int64, lowerBound, upperBound float64, _ bool, hostname string, tags []string, _ bool) {
	c.record("openmetrics_bucket", metric, float64(value), hostname, append(tags,
		fmt.Sprintf("lower_bound:%g", lowerBound),
		fmt.Sprintf("upper_bound:%g", upperBound),
	))
}

func (c *CaptureSender) HistogramBucket(metric string, value int64, lowerBound, upperBound float64, _ bool, hostname string, tags []string, _ bool) {
	c.record("histogram_bucket", metric, float64(value), hostname, append(tags,
		fmt.Sprintf("lower_bound:%g", lowerBound),
		fmt.Sprintf("upper_bound:%g", upperBound),
	))
}

func (c *CaptureSender) GaugeWithTimestamp(metric string, value float64, hostname string, tags []string, timestamp float64) error {
	c.record("gauge_with_timestamp", metric, value, hostname, append(tags, fmt.Sprintf("timestamp:%g", timestamp)))
	return nil
}

func (c *CaptureSender) CountWithTimestamp(metric string, value float64, hostname string, tags []string, timestamp float64) error {
	c.record("count_with_timestamp", metric, value, hostname, append(tags, fmt.Sprintf("timestamp:%g", timestamp)))
	return nil
}

func (c *CaptureSender) Event(_ event.Event) {}

func (c *CaptureSender) EventPlatformEvent(rawEvent []byte, eventType string) {
	if c == nil {
		return
	}
	c.metadata = append(c.metadata, CapturedMetadataEvent{
		EventType: eventType,
		Payload:   append([]byte(nil), rawEvent...),
	})
}

func (c *CaptureSender) GetSenderStats() stats.SenderStats { return stats.SenderStats{} }

func (c *CaptureSender) DisableDefaultHostname(_ bool) {}

func (c *CaptureSender) SetCheckCustomTags(_ []string) {}

func (c *CaptureSender) SetInfraTagger(_ *infratags.Tagger) {}

func (c *CaptureSender) SetCheckService(_ string) {}

func (c *CaptureSender) SetNoIndex(_ bool) {}

func (c *CaptureSender) FinalizeCheckServiceTag() {}

func (c *CaptureSender) OrchestratorMetadata(_ []types.ProcessMessageBody, _ string, _ int) {}

func (c *CaptureSender) OrchestratorManifest(_ []types.ProcessMessageBody, _ string) {}

// FormatCapturedMetric formats a captured metric for CLI output.
func FormatCapturedMetric(metric CapturedMetric) string {
	tags := append([]string(nil), metric.Tags...)
	sort.Strings(tags)
	host := metric.Hostname
	if host == "" {
		host = "-"
	}
	return fmt.Sprintf("%s %s %g host=%s tags=[%s]", metric.Type, metric.Name, metric.Value, host, strings.Join(tags, ","))
}

// FormatCapturedMetadataEvent formats a metadata payload for CLI output.
func FormatCapturedMetadataEvent(event CapturedMetadataEvent) string {
	if len(event.Payload) == 0 {
		return fmt.Sprintf("metadata event_type=%s payload=<empty>", event.EventType)
	}

	var formatted interface{}
	if err := json.Unmarshal(event.Payload, &formatted); err != nil {
		return fmt.Sprintf("metadata event_type=%s payload=%s", event.EventType, string(event.Payload))
	}

	pretty, err := json.MarshalIndent(formatted, "", "  ")
	if err != nil {
		return fmt.Sprintf("metadata event_type=%s payload=%s", event.EventType, string(event.Payload))
	}
	return fmt.Sprintf("metadata event_type=%s\n%s", event.EventType, string(pretty))
}
