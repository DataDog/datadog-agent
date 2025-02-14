// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package subscribers

import (
	"sync/atomic"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type GPUSubscriber struct {
	gpuDetected atomic.Bool
	gpuEventsCh chan workloadmeta.EventBundle
	stopCh      chan struct{}
	wmeta       workloadmeta.Component
}

// NewGPUSubscriber creates a new GPUDetector instance
func NewGPUSubscriber(wmeta workloadmeta.Component) *GPUSubscriber {
	return &GPUSubscriber{
		stopCh: make(chan struct{}),
		wmeta:  wmeta,
	}
}

// Run starts the GPU detector, which listens for workloadmeta events to detect GPUs on the host
func (g *GPUSubscriber) Run() {
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
			g.processEvents(eventBundle)
			eventBundle.Acknowledge()

			if g.IsGPUDetected() {
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
		gpu, ok := event.Entity.(*workloadmeta.GPU)
		if !ok {
			log.Debugf("Expected workloadmeta.GPU got %T, skipping", event.Entity)
			continue
		}

		log.Info("GPU detected, enabling GPU tagging:", gpu)
		g.SetGPUDetected(true)
		break
	}
}

// Stop stops the GPU detector
func (g *GPUSubscriber) Stop() {
	close(g.stopCh)
}
