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
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
)

const (
	processTaggerSubsystem = "nccl"
	processTaggerCacheSize = 4096
)

// ProcessTagger handles PID -> container -> pod tag correlation
type ProcessTagger struct {
	cache *gpu.WorkloadTagCache
}

// NewProcessTagger creates a new ProcessTagger
func NewProcessTagger(taggerComp tagger.Component, wmeta workloadmeta.Component, containerProvider proccontainers.ContainerProvider, tm telemetry.Component) *ProcessTagger {
	pt := &ProcessTagger{}
	if taggerComp == nil || wmeta == nil || tm == nil {
		return pt
	}
	cache, err := gpu.NewWorkloadTagCacheWithSubsystem(processTaggerSubsystem, taggerComp, wmeta, containerProvider, tm, processTaggerCacheSize)
	if err != nil {
		return pt
	}
	pt.cache = cache
	return pt
}

// SetContainerProvider sets the container provider after construction.
func (pt *ProcessTagger) SetContainerProvider(p proccontainers.ContainerProvider) {
	if pt.cache == nil {
		return
	}
	pt.cache.SetContainerProvider(p)
}

// GetTagsForPID returns tags for a given PID by correlating to container/pod
func (pt *ProcessTagger) GetTagsForPID(pid int) ([]string, error) {
	pidTag := fmt.Sprintf("pid:%d", pid)

	if pt.cache == nil {
		return []string{pidTag}, nil
	}

	workloadID := workloadmeta.EntityID{
		Kind: workloadmeta.KindProcess,
		ID:   strconv.Itoa(pid),
	}
	tags, err := pt.cache.GetOrCreateWorkloadTags(workloadID)
	if len(tags) == 0 {
		return []string{pidTag}, err
	}
	return tags, err
}

// Refresh clears the PID → container cache
func (pt *ProcessTagger) Refresh() {
	if pt.cache == nil {
		return
	}
	pt.cache.MarkStale()
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
	case workloadmeta.KindProcess:
		pid, err := strconv.Atoi(workloadID.ID)
		if err != nil {
			return nil, fmt.Errorf("invalid process ID: %w", err)
		}
		return pt.GetTagsForPID(pid)
	case workloadmeta.KindContainer:
		if pt.cache == nil {
			return nil, errors.New("workload tag cache not initialized")
		}
		return pt.cache.GetOrCreateWorkloadTags(workloadID)
	default:
		return nil, fmt.Errorf("unsupported workload kind: %s", workloadID.Kind)
	}
}
