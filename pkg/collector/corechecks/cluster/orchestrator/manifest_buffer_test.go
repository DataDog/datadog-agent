// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator
// +build kubeapiserver,orchestrator

package orchestrator

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gotest.tools/assert"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer"
)

// TestOrchestratorManifestBuffer tests manifest buffer
// This test sets maximum beffer size to 2 and we have 5 manifests in total
// The maximum manifests per message is 100 by default
// Therefore we should have 3 messages to send
// message 1 contains manifest 1 and 2
// message 2 contains manifest 3 and 4
// message 3 contains manifest 5
func TestOrchestratorManifestBuffer(t *testing.T) {
	orchCheck := OrchestratorFactory().(*OrchestratorCheck)
	mb := NewManifestBuffer(orchCheck)
	mb.Cfg.MaxBufferedManifests = 2
	mb.Cfg.ManifestBufferFlushInterval = 3 * time.Second

	noopSender := noopSender{}
	mb.Start(&noopSender)

	// Send 5 manifests to the buffer
	for i := 1; i <= 5; i++ {
		mb.ManifestChan <- &model.Manifest{
			Type: int32(i),
		}
	}

	mb.Stop()

	// The buffer should be flushed 3 times
	require.Len(t, noopSender.OrchestratorManifestOut, 3)

	i := int32(1)
	for _, collectorManifests := range noopSender.OrchestratorManifestOut {
		// The maximum manifests per message is 100 by default, therefore we should only have one message created by each flush
		require.Len(t, collectorManifests, 1)
		collectorManifest := collectorManifests[0].(*model.CollectorManifest)
		for _, m := range collectorManifest.Manifests {
			assert.Equal(t, i, m.Type)
			i++
		}
	}

}

type noopSender struct {
	OrchestratorManifestOut [][]model.MessageBody
}

var _ aggregator.Sender = &noopSender{}

func (s *noopSender) OrchestratorManifest(msgs []serializer.ProcessMessageBody, clusterID string) {
	s.OrchestratorManifestOut = append(s.OrchestratorManifestOut, msgs)
}

func (s *noopSender) DisableDefaultHostname(_ bool) {
	panic("implement me ")
}

func (s *noopSender) SetCheckCustomTags(_ []string) {
	panic("implement me ")
}

func (s *noopSender) SetCheckService(_ string) {
	panic("implement me ")
}

func (s *noopSender) FinalizeCheckServiceTag() {
	panic("implement me ")
}

func (s *noopSender) Commit() {
	panic("implement me ")
}

func (s *noopSender) GetSenderStats() (metricStats check.SenderStats) {
	panic("implement me ")
}

func (s *noopSender) SendRawMetricSample(sample *metrics.MetricSample) {
	panic("implement me ")
}

func (s *noopSender) Gauge(_ string, _ float64, _ string, _ []string) {
	panic("implement me ")
}

func (s *noopSender) GaugeNoIndex(_ string, _ float64, _ string, _ []string) {
	panic("implement me ")
}

func (s *noopSender) Rate(_ string, _ float64, _ string, _ []string) {
	panic("implement me ")
}

func (s *noopSender) Count(_ string, _ float64, _ string, _ []string) {
	panic("implement me ")
}

func (s *noopSender) MonotonicCount(_ string, _ float64, _ string, _ []string) {
	panic("implement me ")
}

func (s *noopSender) MonotonicCountWithFlushFirstValue(_ string, _ float64, _ string, _ []string, _ bool) {
	panic("implement me ")
}

func (s *noopSender) Counter(_ string, _ float64, _ string, _ []string) {
	panic("implement me ")
}

func (s *noopSender) Histogram(_ string, _ float64, _ string, _ []string) {
	panic("implement me ")
}

func (s *noopSender) HistogramBucket(_ string, _ int64, _, _ float64, _ bool, _ string, _ []string, _ bool) {
	panic("implement me ")
}

func (s *noopSender) Historate(_ string, _ float64, _ string, _ []string) {
	panic("implement me ")
}

func (s *noopSender) SendRawServiceCheck(_ *metrics.ServiceCheck) {
	panic("implement me ")
}

func (s *noopSender) ServiceCheck(_ string, _ metrics.ServiceCheckStatus, _ string, _ []string, _ string) {
	panic("implement me ")
}

func (s *noopSender) Event(_ metrics.Event) {
	panic("implement me ")
}

func (s *noopSender) EventPlatformEvent(_ string, _ string) {
	panic("implement me ")
}

func (s *noopSender) OrchestratorMetadata(_ []serializer.ProcessMessageBody, _ string, _ int) {
	panic("implement me ")
}

func (s *noopSender) ContainerLifecycleEvent(_ []serializer.ContainerLifecycleMessage) {
	panic("implement me ")
}
