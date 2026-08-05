// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package aggregator

import "github.com/DataDog/datadog-agent/pkg/metrics"

// FinalDogStatsDSerieObserver observes final DogStatsD series from the normal
// aggregation pipeline only. The pipeline calls observers synchronously after
// its aggregation filter and before serialization. Implementations must not
// mutate or retain the series, must tolerate concurrent calls, and must return
// promptly because they run on the aggregation hot path.
type FinalDogStatsDSerieObserver interface {
	ObserveFinalDogStatsDSerie(serie *metrics.Serie)
}
