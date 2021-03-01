// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-go/statsd"
)

// ReordererMonitor represents a reorderer monitor
type ReordererMonitor struct {
	// probe is a pointer to the Probe
	probe *Probe
	// statsdClient is a pointer to the statsdClient used to report the metrics of the perf buffer monitor
	statsdClient *statsd.Client
}

// NewReOrderMonitor instantiates a new reorder statistics counter
func NewReOrderMonitor(p *Probe, client *statsd.Client) (*ReordererMonitor, error) {
	return &ReordererMonitor{
		probe:        p,
		statsdClient: client,
	}, nil
}

// Start the reorderer monitor
func (r *ReordererMonitor) Start(ctx context.Context) {
	for {
		select {
		case metric := <-r.probe.reOrderer.Metrics:
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
