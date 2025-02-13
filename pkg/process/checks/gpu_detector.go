// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"sync/atomic"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type GPUDetector struct {
	detectedGPU atomic.Bool
	gpuEventsCh chan workloadmeta.EventBundle
	stopCh      chan struct{}
	wmeta       workloadmeta.Component
}

func NewGPUDetector(wmeta workloadmeta.Component) *GPUDetector {
	return &GPUDetector{
		stopCh: make(chan struct{}),
		wmeta:  wmeta,
	}
}

// Run starts the GPU detector, which listens for workloadmeta events to detect GPUs on the host
func (g *GPUDetector) Run() {
	filter := workloadmeta.NewFilterBuilder().
		SetSource(workloadmeta.SourceRuntime).
		SetEventType(workloadmeta.EventTypeSet).
		AddKind(workloadmeta.KindGPU).
		Build()

	g.gpuEventsCh = g.wmeta.Subscribe(
		"gpu-detector-set-gpu",
		workloadmeta.NormalPriority,
		filter,
	)
	for {
		select {
		case eventBundle, ok := <-g.gpuEventsCh:
			if !ok {
				return
			}
			for _, event := range eventBundle.Events {
				gpu, ok := event.Entity.(*workloadmeta.GPU)
				if !ok {
					log.Debugf("Expected workloadmeta.GPU got %T, skipping", event.Entity)
					continue
				}

				log.Info("GPU detected, enabling GPU tagging:", gpu)
				g.detectedGPU.Store(true)
				break
			}
			eventBundle.Acknowledge()

			if g.detectedGPU.Load() {
				log.Info("GPU detected in event bundle")
			} else {
				log.Info("GPU not detected in event bundle, continuing to listen")
			}
		case <-g.stopCh:
			g.wmeta.Unsubscribe(g.gpuEventsCh)
			return
		}
	}
}

// GetGPUTags creates and returns a mapping of active pids to their associated GPU tags
func (g *GPUDetector) GetGPUTags() map[int32][]string {
	if !g.detectedGPU.Load() {
		log.Info("GPU not detected, skipping GPU tag creation")
		return nil
	}

	wmetaGPUs := g.wmeta.ListGPUs()

	pidToGPUTags := make(map[int32][]string)
	for _, gpu := range wmetaGPUs {
		gpuTags := []string{
			"gpu_uuid:" + gpu.ID,
			"gpu_device:" + gpu.Device,
			"gpu_vendor:" + gpu.Vendor,
		}

		// An active pid can be associated with multiple GPUs
		for _, pid := range gpu.ActivePIDs {
			pidToGPUTags[int32(pid)] = append(pidToGPUTags[int32(pid)], gpuTags...)
		}
	}
	return pidToGPUTags
}

func (g *GPUDetector) Stop() {
	close(g.stopCh)
}
