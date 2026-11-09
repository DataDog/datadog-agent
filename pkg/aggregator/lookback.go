// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer"
)

// DogStatsDLookbackContext carries the effective series identity resolved by
// the normal DogStatsD aggregation path. Lookback implementations should use
// this context instead of reimplementing DogStatsD tagger enrichment, host
// extraction, metric tag filtering, source, or no-index behavior.
type DogStatsDLookbackContext struct {
	// ContextKey is the same context key used by the TimeSampler for the normal
	// DogStatsD aggregation path. It lets lookback implementations avoid
	// rebuilding a selected series key on every matching sample.
	ContextKey ckey.ContextKey

	Name    string
	Host    string
	Tags    []string
	NoIndex bool
	Source  metrics.MetricSource
}

// DogStatsDLookback receives selected semantic DogStatsD observations from the
// existing aggregation pipeline. Concrete implementations live outside
// pkg/aggregator so binaries that do not enable metric lookback do not link the
// retention ring or trigger code.
type DogStatsDLookback interface {
	// WantsDogStatsDMetric is the hot-path exact-name admission check. It must not
	// allocate for nonmatching samples.
	WantsDogStatsDMetric(name string) bool

	// ObserveDogStatsDSample receives a normal DogStatsD sample after the existing
	// TimeSampler context resolver has computed the effective series identity and
	// after normal aggregation has accepted the sample.
	ObserveDogStatsDSample(sample *metrics.MetricSample, timestamp float64, ctx DogStatsDLookbackContext)

	// FlushDogStatsDBuckets gives the lookback materializer a chance to seal
	// buckets using the same clock cadence as normal DogStatsD flushing. When
	// forceFlushAll is true, open buckets should be sealed even if they are newer
	// than the normal lookback seal delay.
	FlushDogStatsDBuckets(timestamp float64, forceFlushAll bool)

	// AppendDogStatsDNoAggSerie receives the same DogStatsD no-aggregation series
	// that the worker appends to the serializer.
	AppendDogStatsDNoAggSerie(*metrics.Serie)
}

// DogStatsDLookbackStopper is optionally implemented by DogStatsDLookback
// adapters that own background resources. The demultiplexer calls Stop during
// shutdown when this interface is present.
type DogStatsDLookbackStopper interface {
	Stop()
}

// DogStatsDLookbackFactory builds a DogStatsD lookback adapter after the
// demultiplexer has constructed its serializer. This keeps dump-trigger wiring
// out of pkg/aggregator while still allowing concrete lookback code to close
// over the serializer it should dump through.
type DogStatsDLookbackFactory func(serializer.MetricSerializer) DogStatsDLookback
