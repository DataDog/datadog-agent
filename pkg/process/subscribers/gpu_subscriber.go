// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package subscribers implements subscribers to workloadmeta.
package subscribers

import (
	"context"
	"sync/atomic"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GPUSubscriber is a subscriber that listens for GPU events from workloadmeta
type GPUSubscriber struct {
	gpuDetected atomic.Bool
	gpuEventsCh chan workloadmeta.EventBundle
	stopCh      chan struct{}
	wmeta       workloadmeta.Component
	tagger      tagger.Component
}

// NewGPUSubscriber creates a new GPUDetector instance
func NewGPUSubscriber(wmeta workloadmeta.Component, tagger tagger.Component) *GPUSubscriber {
	return &GPUSubscriber{
		stopCh: make(chan struct{}),
		wmeta:  wmeta,
		tagger: tagger,
	}
}

// Run starts the GPU detector, which listens for workloadmeta events to detect GPUs on the host
func (g *GPUSubscriber) Run(_ context.Context) error {
	log.Info("Starting GPU subscriber")
	filter := workloadmeta.NewFilterBuilder().
		SetSource(workloadmeta.SourceRuntime).
		SetEventType(workloadmeta.EventTypeSet).
		AddKind(workloadmeta.KindGPU).
		Build()

	g.gpuEventsCh = g.wmeta.Subscribe(
		"gpu-subscriber-set-gpu",
		workloadmeta.NormalPriority,
		filter,
	)
	go func() {
		for {
			select {
			case eventBundle, ok := <-g.gpuEventsCh:
				if !ok {
					return
				}
				g.processEvents(eventBundle)
				eventBundle.Acknowledge()
			case <-g.stopCh:
				g.wmeta.Unsubscribe(g.gpuEventsCh)
				return
			}
		}
	}()

	return nil
}

// IsGPUDetected checks if a GPU has been detected
func (g *GPUSubscriber) IsGPUDetected() bool {
	return g.gpuDetected.Load()
}

// SetGPUDetected sets the GPU detected status
func (g *GPUSubscriber) SetGPUDetected(value bool) {
	g.gpuDetected.Store(value)
}

// processEvents processes the events received from workloadmeta
func (g *GPUSubscriber) processEvents(eventBundle workloadmeta.EventBundle) {
	for _, event := range eventBundle.Events {
		if _, ok := event.Entity.(*workloadmeta.GPU); ok {
			g.SetGPUDetected(true)
			break
		}
	}
}

// GetGPUTags creates and returns a mapping of active pids to their associated GPU tags
func (g *GPUSubscriber) GetGPUTags() map[int32][]string {
	if !g.IsGPUDetected() {
		return map[int32][]string{}
	}

	wmetaGPUs := g.wmeta.ListGPUs()

	pidToTagSet := make(map[int32]common.StringSet)
	for _, gpu := range wmetaGPUs {
		uuid := gpu.ID

		// Use tagger to get gpu tags
		entityID := types.NewEntityID(types.GPU, uuid)
		tags, err := g.tagger.Tag(entityID, types.ChecksConfigCardinality)
		if err != nil {
			log.Debugf("Could not collect tags for GPU %s, err: %v", uuid, err)
			continue
		}

		// Filter tags to remove duplicates
		for _, pid := range gpu.ActivePIDs {
			if _, ok := pidToTagSet[int32(pid)]; !ok {
				pidToTagSet[int32(pid)] = common.NewStringSet()
			}
			for _, tag := range tags {
				pidToTagSet[int32(pid)].Add(tag)
			}
		}
	}

	// Convert StringSet to []string
	pidToGPUTags := make(map[int32][]string, len(pidToTagSet))
	for pid, tagSet := range pidToTagSet {
		pidToGPUTags[pid] = tagSet.GetAll()
	}

	return pidToGPUTags
}

// Stop stops the GPU detector
func (g *GPUSubscriber) Stop(_ context.Context) error {
	close(g.stopCh)
	return nil
}
