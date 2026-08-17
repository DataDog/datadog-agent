// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package aggregator

import "github.com/DataDog/datadog-agent/pkg/metrics"

// FinalDogStatsDSerieObserver observes final DogStatsD series from the normal
// aggregation pipeline only. The pipeline calls observers synchronously after
// its aggregation filter and before serialization. Worker flushes are serialized
// by the demultiplexer. Implementations must not mutate or retain the series and
// must return promptly because they run on the aggregation hot path.
type FinalDogStatsDSerieObserver interface {
	ObserveFinalDogStatsDSerie(serie *metrics.Serie)
}

// FinalDogStatsDSerieObserverFlusher is implemented by observers that evaluate
// a completed serializer-flush window. The demultiplexer invokes it once after
// all normal-aggregation DogStatsD workers have finished flushing. It is called
// synchronously and must return promptly.
type FinalDogStatsDSerieObserverFlusher interface {
	CompleteFinalDogStatsDSerieFlush()
}

func completeFinalDogStatsDSerieObserverFlushes(observers []FinalDogStatsDSerieObserver) {
	for _, observer := range observers {
		if flusher, ok := observer.(FinalDogStatsDSerieObserverFlusher); ok {
			flusher.CompleteFinalDogStatsDSerieFlush()
		}
	}
}
