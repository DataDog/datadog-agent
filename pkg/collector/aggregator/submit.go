// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	metricsevent "github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Metric type values. They match the ordering of rtloader's metric_type_t enum
// (used by Python checks) and of the shared-library check ABI (used by Rust
// checks), so both can submit metrics through the same code path.
const (
	metricTypeGauge = iota
	metricTypeRate
	metricTypeCount
	metricTypeMonotonicCount
	metricTypeCounter
	metricTypeHistogram
	metricTypeHistorate
)

// senderForCheck returns the sender associated with the given check ID from the
// global check context. Both Python and shared-library checks resolve their
// sender this way, so the check ID is the only routing information required.
func senderForCheck(checkID string) (sender.Sender, error) {
	checkContext, err := GetCheckContext()
	if err != nil {
		return nil, err
	}
	// Use CheckContext.GetSender so per-check sender-manager overrides are
	// honored (e.g. routing Python shadow checks to the lookback sender).
	return checkContext.GetSender(checkid.ID(checkID))
}

// SubmitMetric submits a metric to the sender associated with checkID.
// metricType uses the rtloader metric_type_t ordering (Gauge=0, Rate=1, ...).
func SubmitMetricForCheck(checkID string, metricType int, name string, value float64, tags []string, hostname string, flushFirstValue bool) {
	s, err := senderForCheck(checkID)
	if err != nil || s == nil {
		log.Errorf("Error submitting metric to the Sender: %v", err)
		return
	}

	switch metricType {
	case metricTypeGauge:
		s.Gauge(name, value, hostname, tags)
	case metricTypeRate:
		s.Rate(name, value, hostname, tags)
	case metricTypeCount:
		s.Count(name, value, hostname, tags)
	case metricTypeMonotonicCount:
		s.MonotonicCountWithFlushFirstValue(name, value, hostname, tags, flushFirstValue)
	case metricTypeCounter:
		s.Counter(name, value, hostname, tags)
	case metricTypeHistogram:
		s.Histogram(name, value, hostname, tags)
	case metricTypeHistorate:
		s.Historate(name, value, hostname, tags)
	}
}

// SubmitServiceCheck submits a service check to the sender associated with checkID.
func SubmitServiceCheckForCheck(checkID string, name string, status servicecheck.ServiceCheckStatus, tags []string, hostname string, message string) {
	s, err := senderForCheck(checkID)
	if err != nil || s == nil {
		log.Errorf("Error submitting service check to the Sender: %v", err)
		return
	}

	s.ServiceCheck(name, status, hostname, tags, message)
}

// SubmitEvent submits an event to the sender associated with checkID.
func SubmitEventForCheck(checkID string, event metricsevent.Event) {
	s, err := senderForCheck(checkID)
	if err != nil || s == nil {
		log.Errorf("Error submitting event to the Sender: %v", err)
		return
	}

	s.Event(event)
}

// SubmitHistogramBucket submits a histogram bucket to the sender associated with checkID.
func SubmitHistogramBucketForCheck(checkID string, name string, value int64, lowerBound float64, upperBound float64, monotonic bool, hostname string, tags []string, flushFirstValue bool) {
	s, err := senderForCheck(checkID)
	if err != nil || s == nil {
		log.Errorf("Error submitting histogram bucket to the Sender: %v", err)
		return
	}

	s.OpenmetricsBucket(name, value, lowerBound, upperBound, monotonic, hostname, tags, flushFirstValue)
}

// SubmitEventPlatformEvent submits an event platform event to the sender associated with checkID.
func SubmitEventPlatformEventForCheck(checkID string, rawEvent []byte, eventType string) {
	s, err := senderForCheck(checkID)
	if err != nil || s == nil {
		log.Errorf("Error submitting event platform event to the Sender: %v", err)
		return
	}

	s.EventPlatformEvent(rawEvent, eventType)
}

// LogMessage logs a message through the agent logger at the given level. The
// level values match rtloader's log_level_t enum (Trace=7, Debug=10, Info=20,
// Warning=30, Error=40, Critical=50) so Python and shared-library checks share
// the same logging behavior.
func LogMessage(level int, message string) {
	switch level {
	case 50: // CRITICAL
		log.Critical(message)
	case 40: // ERROR
		log.Error(message)
	case 30: // WARNING
		log.Warn(message)
	case 20: // INFO
		log.Info(message)
	case 10: // DEBUG
		log.Debug(message)
	case 7: // TRACE
		log.Trace(message)
	default: // unknown log level
		log.Info(message)
	}
}
