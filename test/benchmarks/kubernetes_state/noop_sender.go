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

// noopSender is a sender.Sender that discards everything. It exists so the
// benchmark can drive KSMCheck.Run() without pulling in the real aggregator
// (channel hop, context resolver, retained series) which would show up as
// noise in the CPU/memory profile we're trying to attribute to the check's
// own logic.
type noopSender struct{}

var _ sender.Sender = noopSender{}

func (noopSender) Commit()                                                                         {}
func (noopSender) Gauge(string, float64, string, []string)                                         {}
func (noopSender) GaugeNoIndex(string, float64, string, []string)                                  {}
func (noopSender) Rate(string, float64, string, []string)                                          {}
func (noopSender) Count(string, float64, string, []string)                                         {}
func (noopSender) MonotonicCount(string, float64, string, []string)                                {}
func (noopSender) MonotonicCountWithFlushFirstValue(string, float64, string, []string, bool)       {}
func (noopSender) Counter(string, float64, string, []string)                                       {}
func (noopSender) Histogram(string, float64, string, []string)                                     {}
func (noopSender) Historate(string, float64, string, []string)                                     {}
func (noopSender) Distribution(string, float64, string, []string)                                  {}
func (noopSender) ServiceCheck(string, servicecheck.ServiceCheckStatus, string, []string, string)  {}
func (noopSender) OpenmetricsBucket(string, int64, float64, float64, bool, string, []string, bool) {}
func (noopSender) HistogramBucket(string, int64, float64, float64, bool, string, []string, bool)   {}
func (noopSender) GaugeWithTimestamp(string, float64, string, []string, float64) error             { return nil }
func (noopSender) CountWithTimestamp(string, float64, string, []string, float64) error             { return nil }
func (noopSender) Event(event.Event)                                                               {}
func (noopSender) EventPlatformEvent([]byte, string)                                               {}
func (noopSender) GetSenderStats() stats.SenderStats                                               { return stats.SenderStats{} }
func (noopSender) DisableDefaultHostname(bool)                                                     {}
func (noopSender) SetCheckCustomTags([]string)                                                     {}
func (noopSender) SetInfraTagger(*infratags.Tagger)                                                {}
func (noopSender) SetCheckService(string)                                                          {}
func (noopSender) SetNoIndex(bool)                                                                 {}
func (noopSender) FinalizeCheckServiceTag()                                                        {}
func (noopSender) OrchestratorMetadata([]serializertypes.ProcessMessageBody, string, int)          {}
func (noopSender) OrchestratorManifest([]serializertypes.ProcessMessageBody, string)               {}

// noopSenderManager is a sender.SenderManager that always hands out the same
// noopSender, regardless of check ID.
type noopSenderManager struct{}

var _ sender.SenderManager = noopSenderManager{}

func (noopSenderManager) GetSender(checkid.ID) (sender.Sender, error) { return noopSender{}, nil }
func (noopSenderManager) SetSender(sender.Sender, checkid.ID) error   { return nil }
func (noopSenderManager) DestroySender(checkid.ID)                    {}
func (noopSenderManager) GetDefaultSender() (sender.Sender, error)    { return noopSender{}, nil }
