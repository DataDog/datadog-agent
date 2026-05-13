// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nccl

import (
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
	cache     *gpu.WorkloadTagCache
	tagger    tagger.Component
	wmeta     workloadmeta.Component
	telemetry telemetry.Component
}

// NewProcessTagger creates a new ProcessTagger
func NewProcessTagger(taggerComp tagger.Component, wmeta workloadmeta.Component, containerProvider proccontainers.ContainerProvider, tm telemetry.Component) *ProcessTagger {
	pt := &ProcessTagger{
		tagger:    taggerComp,
		wmeta:     wmeta,
		telemetry: tm,
	}
	if taggerComp == nil || wmeta == nil || tm == nil {
		return pt
	}
	cache, err := gpu.NewWorkloadTagCache(taggerComp, wmeta, containerProvider, tm, processTaggerCacheSize)
	if err != nil {
		return pt
	}
	pt.cache = cache
	return pt
}

// SetContainerProvider rebuilds the cache with the given container provider.
// Called lazily after construction once the shared provider becomes available.
func (pt *ProcessTagger) SetContainerProvider(p proccontainers.ContainerProvider) {
	if pt.tagger == nil || pt.wmeta == nil || pt.telemetry == nil || p == nil {
		return
	}
	cache, err := gpu.NewWorkloadTagCache(pt.tagger, pt.wmeta, p, pt.telemetry, processTaggerCacheSize)
	if err != nil {
		return
	}
	pt.cache = cache
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
