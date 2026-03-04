// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package nccl

import (
	"errors"
	"fmt"
	"strconv"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	agenterrors "github.com/DataDog/datadog-agent/pkg/errors"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
)

// ProcessTagger handles PID -> container -> pod tag correlation
// This is a simplified version of the GPU check's WorkloadTagCache,
// without the nvml build tag dependency
type ProcessTagger struct {
	tagger            tagger.Component
	wmeta             workloadmeta.Component
	containerProvider proccontainers.ContainerProvider
	pidToCid          map[int]string
}

// NewProcessTagger creates a new ProcessTagger
func NewProcessTagger(tagger tagger.Component, wmeta workloadmeta.Component, containerProvider proccontainers.ContainerProvider) *ProcessTagger {
	return &ProcessTagger{
		tagger:            tagger,
		wmeta:             wmeta,
		containerProvider: containerProvider,
	}
}

// GetTagsForPID returns tags for a given PID by correlating to container/pod
func (pt *ProcessTagger) GetTagsForPID(pid int) ([]string, error) {
	tags := []string{fmt.Sprintf("pid:%d", pid)}

	// Try to get container ID from workloadmeta process data
	var containerID string
	if pt.wmeta != nil {
		process, err := pt.wmeta.GetProcess(int32(pid))
		if err == nil {
			if process.Owner != nil && process.Owner.Kind == workloadmeta.KindContainer {
				containerID = process.Owner.ID
			}
		} else if agenterrors.IsNotFound(err) {
			// Fall back to container provider
			containerID, _ = pt.getContainerID(pid)
		}
	} else {
		// Fall back to container provider if wmeta is not available
		containerID, _ = pt.getContainerID(pid)
	}

	if containerID == "" {
		return tags, nil
	}

	// Get container tags from tagger
	containerTags, err := pt.getContainerTags(containerID)
	if err != nil && !agenterrors.IsNotFound(err) {
		return tags, fmt.Errorf("error getting container tags: %w", err)
	}
	tags = append(tags, containerTags...)

	return tags, nil
}

// getContainerID retrieves the container ID for a given PID from the container provider
func (pt *ProcessTagger) getContainerID(pid int) (string, error) {
	if pt.containerProvider == nil {
		return "", errors.New("no container provider available")
	}

	if pt.pidToCid == nil {
		pt.pidToCid = pt.containerProvider.GetPidToCid(0)
	}

	containerID, exists := pt.pidToCid[pid]
	if !exists {
		return "", agenterrors.NewNotFound(pid)
	}

	return containerID, nil
}

// getContainerTags builds the tags for a container
func (pt *ProcessTagger) getContainerTags(containerID string) ([]string, error) {
	if pt.wmeta == nil || pt.tagger == nil {
		return nil, errors.New("workloadmeta or tagger not available")
	}

	container, err := pt.wmeta.GetContainer(containerID)
	if err != nil {
		return nil, fmt.Errorf("error getting container %s: %w", containerID, err)
	}

	entityID := taggertypes.NewEntityID(taggertypes.ContainerID, containerID)

	// Use orchestrator cardinality to get pod_name tag
	cardinality := taggertypes.OrchestratorCardinality
	if container.Runtime == workloadmeta.ContainerRuntimeDocker {
		cardinality = taggertypes.HighCardinality
	}

	return pt.tagger.Tag(entityID, cardinality)
}

// Refresh clears the PID → container cache
func (pt *ProcessTagger) Refresh() {
	pt.pidToCid = nil
}

// GetTagsForPIDWithGPU returns tags for a given PID plus GPU-specific tags
func (pt *ProcessTagger) GetTagsForPIDWithGPU(pid int, gpuUUID string) ([]string, error) {
	tags, err := pt.GetTagsForPID(pid)
	if gpuUUID != "" {
		tags = append(tags, "gpu_uuid:"+gpuUUID)
	}
	return tags, err
}

// GetWorkloadTagsForPID is an alias for compatibility
func (pt *ProcessTagger) GetWorkloadTagsForPID(pid int) ([]string, error) {
	workloadID := workloadmeta.EntityID{
		Kind: workloadmeta.KindProcess,
		ID:   strconv.Itoa(pid),
	}
	return pt.GetWorkloadTags(workloadID)
}

// GetWorkloadTags retrieves tags for a workload entity
func (pt *ProcessTagger) GetWorkloadTags(workloadID workloadmeta.EntityID) ([]string, error) {
	switch workloadID.Kind {
	case workloadmeta.KindContainer:
		return pt.getContainerTags(workloadID.ID)
	case workloadmeta.KindProcess:
		pid, err := strconv.Atoi(workloadID.ID)
		if err != nil {
			return nil, fmt.Errorf("invalid process ID: %w", err)
		}
		return pt.GetTagsForPID(pid)
	default:
		return nil, fmt.Errorf("unsupported workload kind: %s", workloadID.Kind)
	}
}
