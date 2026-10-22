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
)

// Sender contains the submission methods used by gNMI reporters.
type Sender interface {
	Gauge(metric string, value float64, hostname string, tags []string)
	Rate(metric string, value float64, hostname string, tags []string)
	MonotonicCount(metric string, value float64, hostname string, tags []string)
	EventPlatformEvent(rawEvent []byte, eventType string)
}

var _ Sender = (*CaptureSender)(nil)

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

// CaptureSender records metric and metadata submissions.
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

func (c *CaptureSender) Rate(metric string, value float64, hostname string, tags []string) {
	c.record("rate", metric, value, hostname, tags)
}

func (c *CaptureSender) MonotonicCount(metric string, value float64, hostname string, tags []string) {
	c.record("monotonic_count", metric, value, hostname, tags)
}

func (c *CaptureSender) EventPlatformEvent(rawEvent []byte, eventType string) {
	if c == nil {
		return
	}
	c.metadata = append(c.metadata, CapturedMetadataEvent{
		EventType: eventType,
		Payload:   append([]byte(nil), rawEvent...),
	})
}

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
