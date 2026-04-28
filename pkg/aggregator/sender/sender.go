// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package sender

import (
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"
)

// Sender allows sending metrics from checks/a check
type Sender interface {
	Commit()
	Gauge(metric string, value float64, hostname string, tags []string)
	GaugeNoIndex(metric string, value float64, hostname string, tags []string)
	Rate(metric string, value float64, hostname string, tags []string)
	Count(metric string, value float64, hostname string, tags []string)
	MonotonicCount(metric string, value float64, hostname string, tags []string)
	MonotonicCountWithFlushFirstValue(metric string, value float64, hostname string, tags []string, flushFirstValue bool)
	Counter(metric string, value float64, hostname string, tags []string)
	Histogram(metric string, value float64, hostname string, tags []string)
	Historate(metric string, value float64, hostname string, tags []string)
	Distribution(metric string, value float64, hostname string, tags []string)
	// DistributionBucket submits an explicit bucket into the distribution sketch path.
	//
	// Arguments:
	//   - metric: metric name
	//   - count: number of samples represented by this bucket; must be > 0
	//   - lowerBound: lower bound of the caller-provided bucket
	//   - upperBound: upper bound of the caller-provided bucket; must be >= lowerBound
	//   - monotonic: if true, count is treated as cumulative and converted to a delta
	//   - hostname: optional host override
	//   - tags: metric tags
	//   - flushFirstValue: if true, the first monotonic value (and reset values) are flushed as-is
	//
	// Use this instead of HistogramBucket when the caller wants explicit bucket
	// counts to map to one weighted sketch value, rather than being spread across
	// multiple internal sketch bins by interpolation.
	DistributionBucket(metric string, count int64, lowerBound, upperBound float64, monotonic bool, hostname string, tags []string, flushFirstValue bool)
	ServiceCheck(checkName string, status servicecheck.ServiceCheckStatus, hostname string, tags []string, message string)
	HistogramBucket(metric string, value int64, lowerBound, upperBound float64, monotonic bool, hostname string, tags []string, flushFirstValue bool)
	// GaugeWithTimestamp reports a new gauge value to the intake with the given timestamp.
	// Gauge time series measure a simple value over time.
	// Unlike Gauge(), each submitted value will be passed to the intake as is, without aggregation. Each time series can have only one value per timestamp.
	// The timestamp is in seconds since epoch (accepts fractional seconds)
	GaugeWithTimestamp(metric string, value float64, hostname string, tags []string, timestamp float64) error
	// CountWithTimestamp reports a new count value to the intake with the given timestamp.
	// Count time series measure how many times something happened in some time period.
	// Unlike Count(), each submitted value will be passed to the intake as is, without aggregation. Each time series can have only one value per timestamp.
	// The timestamp is in seconds since epoch (accepts fractional seconds)
	CountWithTimestamp(metric string, value float64, hostname string, tags []string, timestamp float64) error
	Event(e event.Event)
	EventPlatformEvent(rawEvent []byte, eventType string)
	GetSenderStats() stats.SenderStats
	DisableDefaultHostname(disable bool)
	SetCheckCustomTags(tags []string)
	SetCheckService(service string)
	SetNoIndex(noIndex bool)
	FinalizeCheckServiceTag()
	OrchestratorMetadata(msgs []types.ProcessMessageBody, clusterID string, nodeType int)
	OrchestratorManifest(msgs []types.ProcessMessageBody, clusterID string)
}

//nolint:revive // TODO(AML) Fix revive linter
type SenderManager interface {
	GetSender(id checkid.ID) (Sender, error)
	SetSender(Sender, checkid.ID) error
	DestroySender(id checkid.ID)
	GetDefaultSender() (Sender, error)
}
