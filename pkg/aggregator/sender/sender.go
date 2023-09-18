// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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
	ServiceCheck(checkName string, status servicecheck.ServiceCheckStatus, hostname string, tags []string, message string)
	HistogramBucket(metric string, value int64, lowerBound, upperBound float64, monotonic bool, hostname string, tags []string, flushFirstValue bool)
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

type SenderManager interface {
	GetSender(id checkid.ID) (Sender, error)
	SetSender(Sender, checkid.ID) error
	DestroySender(id checkid.ID)
	GetDefaultSender() (Sender, error)
}
