// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package serverimpl

import (
	"strings"
	"time"

	taggerdef "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	coretaggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/gpu/jobmetadata"
	metricsevent "github.com/DataDog/datadog-agent/pkg/metrics/event"
	pkgtaggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
)

const (
	gpuJobMetadataEnabledConfig        = "gpu.job_metadata.enabled"
	gpuJobMetadataTTLConfig            = "gpu.job_metadata.ttl"
	gpuJobMetadataTaggerSource         = "dogstatsd-gpu-job-metadata"
	gpuJobMetadataProcessSweepInterval = 15 * time.Second
	containerIDOriginPrefix            = "container_id://"
)

type gpuJobMetadataProcessRecord struct {
	ContainerID string
	ProcessID   uint32
	Sequence    uint64
}

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

	if record.Action == jobmetadata.ActionEnd {
		s.forgetGPUJobMetadataProcess(record.ContainerID)
		s.clearGPUJobMetadataTags(processor, record.ContainerID)
		return true
	}

	entityID := coretaggertypes.NewEntityID(coretaggertypes.ContainerID, record.ContainerID)
	processor.ProcessTagInfo([]*coretaggertypes.TagInfo{
		{
			Source:                     gpuJobMetadataTaggerSource,
			EntityID:                   entityID,
			OrchestratorCardTags:       record.Tags,
			ExpiryDate:                 record.ExpiresAt,
			PreserveEntityCompleteness: true,
		},
	})
	// UDS origin detection provides the sender PID. Track it so a missing
	// explicit end event can still be cleaned up when the training process exits.
	s.trackGPUJobMetadataProcess(record.ContainerID, event.OriginInfo.LocalData.ProcessID)

	s.log.Infof(
		"DogStatsD GPU job metadata published: entity_id=%q job_id=%q tags=%v expires_at=%s",
		entityID.String(),
		record.JobID,
		record.Tags,
		formatGPUJobMetadataExpiry(record.ExpiresAt),
	)
	return true
}

func (s *dsdServer) startGPUJobMetadataProcessSweeper() {
	if !s.config.GetBool(gpuJobMetadataEnabledConfig) {
		return
	}

	ticker := time.NewTicker(gpuJobMetadataProcessSweepInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.sweepGPUJobMetadataProcesses()
			case <-s.stopChan:
				return
			}
		}
	}()
}

func (s *dsdServer) trackGPUJobMetadataProcess(containerID string, processID uint32) {
	s.gpuJobMetadataProcessesLock.Lock()
	defer s.gpuJobMetadataProcessesLock.Unlock()

	if processID == 0 {
		delete(s.gpuJobMetadataProcesses, containerID)
		return
	}
	if s.gpuJobMetadataProcesses == nil {
		s.gpuJobMetadataProcesses = make(map[string]gpuJobMetadataProcessRecord)
	}

	s.gpuJobMetadataProcessSequence++
	s.gpuJobMetadataProcesses[containerID] = gpuJobMetadataProcessRecord{
		ContainerID: containerID,
		ProcessID:   processID,
		Sequence:    s.gpuJobMetadataProcessSequence,
	}
}

func (s *dsdServer) forgetGPUJobMetadataProcess(containerID string) {
	s.gpuJobMetadataProcessesLock.Lock()
	defer s.gpuJobMetadataProcessesLock.Unlock()
	delete(s.gpuJobMetadataProcesses, containerID)
}

func (s *dsdServer) gpuJobMetadataProcessRecords() []gpuJobMetadataProcessRecord {
	s.gpuJobMetadataProcessesLock.Lock()
	defer s.gpuJobMetadataProcessesLock.Unlock()

	records := make([]gpuJobMetadataProcessRecord, 0, len(s.gpuJobMetadataProcesses))
	for _, record := range s.gpuJobMetadataProcesses {
		records = append(records, record)
	}
	return records
}

func (s *dsdServer) removeGPUJobMetadataProcessIfCurrent(record gpuJobMetadataProcessRecord) bool {
	s.gpuJobMetadataProcessesLock.Lock()
	defer s.gpuJobMetadataProcessesLock.Unlock()

	current, ok := s.gpuJobMetadataProcesses[record.ContainerID]
	if !ok || current.ProcessID != record.ProcessID || current.Sequence != record.Sequence {
		return false
	}
	delete(s.gpuJobMetadataProcesses, record.ContainerID)
	return true
}

func (s *dsdServer) sweepGPUJobMetadataProcesses() {
	processor, ok := s.taggerProcessor.Get()
	if !ok {
		return
	}

	processExists := s.gpuJobMetadataProcessExists
	if processExists == nil {
		processExists = defaultGPUJobMetadataProcessExists
	}

	for _, record := range s.gpuJobMetadataProcessRecords() {
		if record.ProcessID == 0 || processExists(record.ProcessID) {
			continue
		}
		if !s.removeGPUJobMetadataProcessIfCurrent(record) {
			continue
		}
		s.clearGPUJobMetadataTags(processor, record.ContainerID)
		s.log.Infof("DogStatsD GPU job metadata auto-cleared: entity_id=%q pid=%d", coretaggertypes.NewEntityID(coretaggertypes.ContainerID, record.ContainerID).String(), record.ProcessID)
	}
}

func (s *dsdServer) clearGPUJobMetadataTags(processor taggerdef.Processor, containerID string) {
	entityID := coretaggertypes.NewEntityID(coretaggertypes.ContainerID, containerID)
	processor.ProcessTagInfo([]*coretaggertypes.TagInfo{
		{
			Source:                     gpuJobMetadataTaggerSource,
			EntityID:                   entityID,
			PreserveEntityCompleteness: true,
		},
	})
	s.log.Infof("DogStatsD GPU job metadata cleared: entity_id=%q", entityID.String())
}

func formatGPUJobMetadataExpiry(expiresAt time.Time) string {
	if expiresAt.IsZero() {
		return "none"
	}
	return expiresAt.Format(time.RFC3339)
}

func containerIDFromOriginInfo(originInfo pkgtaggertypes.OriginInfo) string {
	return containerIDFromSocketOrigin(originInfo.ContainerIDFromSocket)
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
