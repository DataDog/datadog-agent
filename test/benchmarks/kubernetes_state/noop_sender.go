// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	serializertypes "github.com/DataDog/datadog-agent/pkg/serializer/types"
	"github.com/DataDog/datadog-agent/pkg/util/infratags"
)

// noopSender is a sender.Sender that discards everything, except for the two gauges KSMCheck.sendTelemetry emits.
type noopSender struct {
	totalMetricsCount   float64
	unknownMetricsCount float64
}

var _ sender.Sender = (*noopSender)(nil)

func (s *noopSender) Gauge(name string, value float64, _ string, _ []string) {
	switch name {
	case "kubernetes_state.telemetry.metrics.count.total":
		s.totalMetricsCount = value
	case "kubernetes_state.telemetry.unknown_metrics.count":
		s.unknownMetricsCount = value
	}
}

func (*noopSender) GaugeNoIndex(string, float64, string, []string)                                  {}
func (*noopSender) Rate(string, float64, string, []string)                                          {}
func (*noopSender) Count(string, float64, string, []string)                                         {}
func (*noopSender) MonotonicCount(string, float64, string, []string)                                {}
func (*noopSender) MonotonicCountWithFlushFirstValue(string, float64, string, []string, bool)       {}
func (*noopSender) Counter(string, float64, string, []string)                                       {}
func (*noopSender) Histogram(string, float64, string, []string)                                     {}
func (*noopSender) Historate(string, float64, string, []string)                                     {}
func (*noopSender) Distribution(string, float64, string, []string)                                  {}
func (*noopSender) ServiceCheck(string, servicecheck.ServiceCheckStatus, string, []string, string)  {}
func (*noopSender) OpenmetricsBucket(string, int64, float64, float64, bool, string, []string, bool) {}
func (*noopSender) HistogramBucket(string, int64, float64, float64, bool, string, []string, bool)   {}
func (*noopSender) GaugeWithTimestamp(string, float64, string, []string, float64) error             { return nil }
func (*noopSender) CountWithTimestamp(string, float64, string, []string, float64) error             { return nil }
func (*noopSender) Event(event.Event)                                                               {}
func (*noopSender) EventPlatformEvent([]byte, string)                                               {}
func (*noopSender) GetSenderStats() stats.SenderStats                                               { return stats.SenderStats{} }
func (*noopSender) DisableDefaultHostname(bool)                                                     {}
func (*noopSender) SetCheckCustomTags([]string)                                                     {}
func (*noopSender) SetInfraTagger(*infratags.Tagger)                                                {}
func (*noopSender) SetCheckService(string)                                                          {}
func (*noopSender) SetNoIndex(bool)                                                                 {}
func (*noopSender) FinalizeCheckServiceTag()                                                        {}
func (*noopSender) OrchestratorMetadata([]serializertypes.ProcessMessageBody, string, int)          {}
func (*noopSender) OrchestratorManifest([]serializertypes.ProcessMessageBody, string)               {}
func (*noopSender) Commit()                                                                         {}

// noopSenderManager is a sender.SenderManager that always hands out the same noopSender instance.
type noopSenderManager struct {
	sender noopSender
}

var _ sender.SenderManager = (*noopSenderManager)(nil)

func (m *noopSenderManager) GetSender(checkid.ID) (sender.Sender, error) { return &m.sender, nil }
func (*noopSenderManager) SetSender(sender.Sender, checkid.ID) error     { return nil }
func (*noopSenderManager) DestroySender(checkid.ID)                      {}
func (m *noopSenderManager) GetDefaultSender() (sender.Sender, error)    { return &m.sender, nil }
