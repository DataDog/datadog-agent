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
	gpuDetected atomic.Bool
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
				g.SetGPUDetected(true)
				break
			}
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

func (g *GPUDetector) IsGPUDetected() bool {
	return g.gpuDetected.Load()
}

func (g *GPUDetector) SetGPUDetected(value bool) {
	g.gpuDetected.Store(value)
}

func (g *GPUDetector) Stop() {
	close(g.stopCh)
}
