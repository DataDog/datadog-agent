// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"context"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
)

// ReordererMonitor represents a reorderer monitor
type ReordererMonitor struct {
	// probe is a pointer to the Probe
	probe *Probe
}

// NewReOrderMonitor instantiates a new reorder statistics counter
func NewReOrderMonitor(p *Probe) (*ReordererMonitor, error) {
	return &ReordererMonitor{
		probe: p,
	}, nil
}

// Start the reorderer monitor
func (r *ReordererMonitor) Start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case metric := <-r.probe.reOrderer.Metrics:
			_ = r.probe.statsdClient.Gauge(metrics.MetricPerfBufferSortingQueueSize, float64(metric.QueueSize), []string{}, 1.0)
			var avg float64
			if metric.TotalOp > 0 {
				avg = float64(metric.TotalDepth) / float64(metric.TotalOp)
			}
			_ = r.probe.statsdClient.Gauge(metrics.MetricPerfBufferSortingAvgOp, avg, []string{}, 1.0)
		case <-ctx.Done():
			return
		}
	}
}
