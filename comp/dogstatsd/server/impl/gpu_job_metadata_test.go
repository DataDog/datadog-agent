// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package serverimpl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	taggerdef "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	coretaggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/listeners"
	pkgtaggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

type fakeTaggerProcessor struct {
	tagInfos []*coretaggertypes.TagInfo
}

func (p *fakeTaggerProcessor) ProcessTagInfo(tagInfos []*coretaggertypes.TagInfo) {
	p.tagInfos = append(p.tagInfos, tagInfos...)
}

func TestGPUJobMetadataEventPublishesOrchestratorTagsFromLocalData(t *testing.T) {
	deps := fulfillDepsWithConfigOverride(t, map[string]interface{}{
		"dogstatsd_port":                    listeners.RandomPortName,
		"dogstatsd_origin_detection_client": true,
		"gpu.job_metadata.enabled":          true,
	})
	s := deps.Server.(*dsdServer)
	processor := &fakeTaggerProcessor{}
	s.taggerProcessor = option.New[taggerdef.Processor](processor)
	requireStart(t, s)

	parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)
	event, err := s.parseEventMessage(parser, []byte("_e{15,5}:datadog.gpu.job|start|s:datadog_gpu_job|c:ci-demo-container|#gpu_job_id:job-123,team:ml"), "", 0)
	require.NoError(t, err)

	require.True(t, s.handleGPUJobMetadataEvent(event))

	require.Len(t, processor.tagInfos, 1)
	info := processor.tagInfos[0]
	require.Equal(t, gpuJobMetadataTaggerSource, info.Source)
	require.Equal(t, coretaggertypes.NewEntityID(coretaggertypes.ContainerID, "demo-container"), info.EntityID)
	require.Equal(t, []string{"gpu_job_id:job-123", "team:ml"}, info.OrchestratorCardTags)
	require.Empty(t, info.LowCardTags)
	require.Empty(t, info.HighCardTags)
	require.True(t, info.IsComplete)
	require.True(t, info.ExpiryDate.IsZero())
}

func TestGPUJobMetadataEventOptionalTTL(t *testing.T) {
	deps := fulfillDepsWithConfigOverride(t, map[string]interface{}{
		"dogstatsd_port":                    listeners.RandomPortName,
		"dogstatsd_origin_detection_client": true,
		"gpu.job_metadata.enabled":          true,
		"gpu.job_metadata.ttl":              time.Minute,
	})
	s := deps.Server.(*dsdServer)
	processor := &fakeTaggerProcessor{}
	s.taggerProcessor = option.New[taggerdef.Processor](processor)
	requireStart(t, s)

	parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)
	event, err := s.parseEventMessage(parser, []byte("_e{15,9}:datadog.gpu.job|heartbeat|s:datadog_gpu_job|c:ci-demo-container|#gpu_job_id:job-123"), "", 0)
	require.NoError(t, err)

	start := time.Now()
	require.True(t, s.handleGPUJobMetadataEvent(event))

	require.Len(t, processor.tagInfos, 1)
	require.WithinDuration(t, start.Add(time.Minute), processor.tagInfos[0].ExpiryDate, time.Second)
}

func TestGPUJobMetadataEndEventClearsTags(t *testing.T) {
	deps := fulfillDepsWithConfigOverride(t, map[string]interface{}{
		"dogstatsd_port":                    listeners.RandomPortName,
		"dogstatsd_origin_detection_client": true,
		"gpu.job_metadata.enabled":          true,
	})
	s := deps.Server.(*dsdServer)
	processor := &fakeTaggerProcessor{}
	s.taggerProcessor = option.New[taggerdef.Processor](processor)
	requireStart(t, s)

	parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)
	event, err := s.parseEventMessage(parser, []byte("_e{15,3}:datadog.gpu.job|end|s:datadog_gpu_job|c:ci-demo-container"), "", 0)
	require.NoError(t, err)

	require.True(t, s.handleGPUJobMetadataEvent(event))

	require.Len(t, processor.tagInfos, 1)
	info := processor.tagInfos[0]
	require.Equal(t, gpuJobMetadataTaggerSource, info.Source)
	require.Equal(t, coretaggertypes.NewEntityID(coretaggertypes.ContainerID, "demo-container"), info.EntityID)
	require.Empty(t, info.OrchestratorCardTags)
	require.Empty(t, info.LowCardTags)
	require.Empty(t, info.HighCardTags)
	require.True(t, info.IsComplete)
}

func TestGPUJobMetadataEventUsesSocketOriginFirst(t *testing.T) {
	originInfo := pkgtaggertypes.OriginInfo{
		ContainerIDFromSocket: "container_id://socket-container",
		LocalData:             origindetection.LocalData{ContainerID: "local-container"},
	}

	require.Equal(t, "socket-container", containerIDFromOriginInfo(originInfo))
}

func TestGPUJobMetadataEventNotConsumedWhenDisabled(t *testing.T) {
	deps := fulfillDepsWithConfigOverride(t, map[string]interface{}{
		"dogstatsd_port":                    listeners.RandomPortName,
		"dogstatsd_origin_detection_client": true,
		"gpu.job_metadata.enabled":          false,
	})
	s := deps.Server.(*dsdServer)
	processor := &fakeTaggerProcessor{}
	s.taggerProcessor = option.New[taggerdef.Processor](processor)
	requireStart(t, s)

	parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)
	event, err := s.parseEventMessage(parser, []byte("_e{15,5}:datadog.gpu.job|start|s:datadog_gpu_job|c:ci-demo-container|#gpu_job_id:job-123"), "", 0)
	require.NoError(t, err)

	require.False(t, s.handleGPUJobMetadataEvent(event))
	require.Empty(t, processor.tagInfos)
}

func TestGPUJobMetadataEventConsumedWhenTaggerProcessorUnavailable(t *testing.T) {
	deps := fulfillDepsWithConfigOverride(t, map[string]interface{}{
		"dogstatsd_port":                    listeners.RandomPortName,
		"dogstatsd_origin_detection_client": true,
		"gpu.job_metadata.enabled":          true,
	})
	s := deps.Server.(*dsdServer)
	requireStart(t, s)

	parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)
	event, err := s.parseEventMessage(parser, []byte("_e{15,5}:datadog.gpu.job|start|s:datadog_gpu_job|c:ci-demo-container|#gpu_job_id:job-123"), "", 0)
	require.NoError(t, err)

	require.True(t, s.handleGPUJobMetadataEvent(event))
}
