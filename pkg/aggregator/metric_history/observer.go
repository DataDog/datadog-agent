// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Integration point (added in Task 4):
//
// In pkg/aggregator/demultiplexer_agent.go:flushToSerializer(), wrap the sink:
//
//   if cfg.MetricHistoryEnabled() {
//       sink = metric_history.NewObservingSink(sink, cache)
//   }

package metric_history

import (
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// ObservingSink wraps a SerieSink to observe metrics before passing them through.
// This allows the MetricHistoryCache to see all metrics without affecting the
// normal pipeline.
type ObservingSink struct {
	delegate metrics.SerieSink
	cache    *MetricHistoryCache
}

// NewObservingSink creates a sink that observes metrics and forwards to delegate.
func NewObservingSink(delegate metrics.SerieSink, cache *MetricHistoryCache) *ObservingSink {
	return &ObservingSink{
		delegate: delegate,
		cache:    cache,
	}
}

// Append implements metrics.SerieSink. It observes the serie in the cache,
// then forwards to the delegate sink.
func (s *ObservingSink) Append(serie *metrics.Serie) {
	if s.cache != nil {
		s.cache.Observe(serie)
	}
	s.delegate.Append(serie)
}
