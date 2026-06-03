// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package serverimpl

import (
	"strings"
	"time"

	coretaggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/gpu/jobmetadata"
	metricsevent "github.com/DataDog/datadog-agent/pkg/metrics/event"
	pkgtaggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
)

const (
	gpuJobMetadataEnabledConfig = "gpu.job_metadata.enabled"
	gpuJobMetadataTTLConfig     = "gpu.job_metadata.ttl"
	gpuJobMetadataTaggerSource  = "dogstatsd-gpu-job-metadata"
	containerIDOriginPrefix     = "container_id://"
)

func (s *dsdServer) handleGPUJobMetadataEvent(event *metricsevent.Event) bool {
	if event == nil || !jobmetadata.IsControlEvent(event.Title, event.SourceTypeName) {
		return false
	}

	if !s.config.GetBool(gpuJobMetadataEnabledConfig) {
		return false
	}

	containerID := containerIDFromOriginInfo(event.OriginInfo)
	record, consumed, err := jobmetadata.RecordFromEvent(containerID, event.Title, event.SourceTypeName, event.Text, event.Tags, s.config.GetDuration(gpuJobMetadataTTLConfig))
	if !consumed {
		return false
	}
	if err != nil {
		s.log.Warnf("DogStatsD GPU job metadata event ignored: %v", err)
		return true
	}

	processor, ok := s.taggerProcessor.Get()
	if !ok {
		s.log.Warn("DogStatsD GPU job metadata event ignored: tagger processor is unavailable")
		return true
	}

	entityID := coretaggertypes.NewEntityID(coretaggertypes.ContainerID, record.ContainerID)
	if record.Action == jobmetadata.ActionEnd {
		processor.ProcessTagInfo([]*coretaggertypes.TagInfo{
			{
				Source:     gpuJobMetadataTaggerSource,
				EntityID:   entityID,
				IsComplete: true,
			},
		})
		s.log.Infof("DogStatsD GPU job metadata cleared: entity_id=%q", entityID.String())
		return true
	}

	processor.ProcessTagInfo([]*coretaggertypes.TagInfo{
		{
			Source:               gpuJobMetadataTaggerSource,
			EntityID:             entityID,
			OrchestratorCardTags: record.Tags,
			ExpiryDate:           record.ExpiresAt,
			IsComplete:           true,
		},
	})

	s.log.Infof(
		"DogStatsD GPU job metadata published: entity_id=%q job_id=%q tags=%v expires_at=%s",
		entityID.String(),
		record.JobID,
		record.Tags,
		formatGPUJobMetadataExpiry(record.ExpiresAt),
	)
	return true
}

func formatGPUJobMetadataExpiry(expiresAt time.Time) string {
	if expiresAt.IsZero() {
		return "none"
	}
	return expiresAt.Format(time.RFC3339)
}

func containerIDFromOriginInfo(originInfo pkgtaggertypes.OriginInfo) string {
	if containerID := containerIDFromSocketOrigin(originInfo.ContainerIDFromSocket); containerID != "" {
		return containerID
	}
	return originInfo.LocalData.ContainerID
}

func containerIDFromSocketOrigin(origin string) string {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return ""
	}
	if strings.HasPrefix(origin, containerIDOriginPrefix) {
		return strings.TrimPrefix(origin, containerIDOriginPrefix)
	}
	if strings.Contains(origin, "://") {
		return ""
	}
	return origin
}
