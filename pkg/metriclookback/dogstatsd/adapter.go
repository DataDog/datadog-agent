// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package dogstatsd adapts DogStatsD aggregation streams to metric lookback retention.
package dogstatsd

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/metriclookback"
	"github.com/DataDog/datadog-agent/pkg/metriclookback/monitor"
	"github.com/DataDog/datadog-agent/pkg/metriclookback/ringbuffer"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// DogStatsDOptions controls selected DogStatsD lookback admission.
type DogStatsDOptions struct {
	// MetricNames is the exact-name allow-list retained from DogStatsD. Non-
	// matching series/samples are ignored without allocation.
	MetricNames []string

	// Monitor is an optional materialized view over the same admitted stream. Its
	// metric name is automatically admitted even if MetricNames is empty.
	Monitor *monitor.Watcher

	// BucketMaterializer receives selected normal DogStatsD samples. It is nil
	// when only no-aggregation final series should be retained.
	BucketMaterializer *DogStatsDBucketMaterializer

	// EgressController is stopped with the adapter when the owning demultiplexer
	// shuts down. It is nil when monitor egress is disabled.
	EgressController *metriclookback.EgressController
}

// DogStatsDAdapter admits selected DogStatsD observations into a shared
// Retention ring and updates an optional monitor view from the same admitted
// stream. Normal DogStatsD samples flow through a bucket materializer before
// retention; no-aggregation series are already final and are retained directly.
type DogStatsDAdapter struct {
	retention        *metriclookback.Retention
	materializer     *DogStatsDBucketMaterializer
	single           string
	names            map[string]struct{}
	monitor          *monitor.Watcher
	egressController *metriclookback.EgressController
}

// NewDogStatsDAdapter creates an exact-name DogStatsD lookback adapter. It
// returns nil when retention is nil or no names are selected.
func NewDogStatsDAdapter(retention *metriclookback.Retention, opts DogStatsDOptions) *DogStatsDAdapter {
	if retention == nil {
		return nil
	}

	seen := make(map[string]struct{}, len(opts.MetricNames)+1)
	for _, name := range opts.MetricNames {
		if name == "" {
			continue
		}
		seen[name] = struct{}{}
	}
	if opts.Monitor != nil {
		if monitorName := opts.Monitor.MetricName(); monitorName != "" {
			seen[monitorName] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil
	}

	adapter := &DogStatsDAdapter{retention: retention, materializer: opts.BucketMaterializer, monitor: opts.Monitor, egressController: opts.EgressController}
	if len(seen) == 1 {
		for name := range seen {
			adapter.single = name
		}
		return adapter
	}

	adapter.names = seen
	return adapter
}

// WantsDogStatsDMetric returns whether name is admitted by the exact-name
// selection used by DogStatsD lookback. It does not allocate for nonmatches.
func (a *DogStatsDAdapter) WantsDogStatsDMetric(name string) bool {
	return a != nil && a.match(name)
}

// ObserveDogStatsDSample admits selected normal DogStatsD samples into the
// bucket materializer. Non-matching samples do not allocate and do not update
// monitor state.
func (a *DogStatsDAdapter) ObserveDogStatsDSample(sample *metrics.MetricSample, timestamp float64, ctx aggregator.DogStatsDLookbackContext) {
	if a == nil || sample == nil || a.materializer == nil || !a.match(sample.Name) {
		return
	}
	a.materializer.Observe(sample, timestamp, ctx)
}

// FlushDogStatsDBuckets seals any selected normal-DogStatsD buckets that are old
// enough to become immutable lookback points.
func (a *DogStatsDAdapter) FlushDogStatsDBuckets(timestamp float64, forceFlushAll bool) {
	if a == nil || a.materializer == nil {
		return
	}
	if forceFlushAll {
		a.materializer.FlushAll(timestamp)
		return
	}
	a.materializer.Flush(timestamp)
}

// Stop stops background resources owned by the adapter.
func (a *DogStatsDAdapter) Stop() {
	if a == nil || a.egressController == nil {
		return
	}
	a.egressController.Stop()
}

// AppendDogStatsDNoAggSerie admits selected no-aggregation series after the
// worker has applied the same tag enrichment, type mapping, and value
// normalization used for normal serialization. Non-matching series do not
// allocate and do not update monitor state.
func (a *DogStatsDAdapter) AppendDogStatsDNoAggSerie(serie *metrics.Serie) {
	if a == nil || serie == nil || len(serie.Points) == 0 || !a.match(serie.Name) {
		return
	}

	_ = a.retention.AppendSerie(context.Background(), ringbuffer.Source{Kind: ringbuffer.SourceDogStatsDNoAggregation}, serie)
	if a.monitor == nil {
		return
	}
	for i := range serie.Points {
		a.monitor.Observe(serie.Name, pointObservedAt(serie.Points[i]))
	}
}

func pointObservedAt(point metrics.Point) time.Time {
	if point.Ts > 0 {
		return time.UnixMicro(int64(point.Ts * 1e6))
	}
	return time.Now()
}

func (a *DogStatsDAdapter) match(name string) bool {
	if a.single != "" {
		return name == a.single
	}
	_, found := a.names[name]
	return found
}
