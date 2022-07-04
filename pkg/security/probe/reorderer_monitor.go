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
	"github.com/DataDog/datadog-go/v5/statsd"
)

// ReordererMonitor represents a reorderer monitor
type ReordererMonitor struct {
	statsdClient statsd.ClientInterface
	reOrderer    *ReOrderer
}

// NewReOrderMonitor instantiates a new reorder statistics counter
func NewReOrderMonitor(statsdClient statsd.ClientInterface, reOrderer *ReOrderer) (*ReordererMonitor, error) {
	return &ReordererMonitor{
		statsdClient: statsdClient,
		reOrderer:    reOrderer,
	}, nil
}

// Start the reorderer monitor
func (r *ReordererMonitor) Start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case metric := <-r.reOrderer.Metrics:
			_ = r.statsdClient.Gauge(metrics.MetricPerfBufferSortingQueueSize, float64(metric.QueueSize), []string{}, 1.0)
			var avg float64
			if metric.TotalOp > 0 {
				avg = float64(metric.TotalDepth) / float64(metric.TotalOp)
			}
			_ = r.statsdClient.Gauge(metrics.MetricPerfBufferSortingAvgOp, avg, []string{}, 1.0)
		case <-ctx.Done():
			return
		}
	}
}
