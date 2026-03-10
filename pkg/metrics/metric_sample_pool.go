// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var (
	tlmMetricSamplePoolGet = telemetry.NewGauge("dogstatsd", "metric_sample_pool_get",
		nil, "Amount of sample gotten from the metric sample pool")
	tlmMetricSamplePoolPut = telemetry.NewGauge("dogstatsd", "metric_sample_pool_put",
		nil, "Amount of sample put in the metric sample pool")
	tlmMetricSamplePool = telemetry.NewGauge("dogstatsd", "metric_sample_pool",
		nil, "Usage of the metric sample pool in dogstatsd")
)

// MetricSampleBatch is a slice of MetricSample. It is used by the MetricSamplePool
// to avoid constant reallocation in high throughput pipelines.
//
// Can be used for both "on-time" and for "late" metrics.
type MetricSampleBatch []MetricSample

// MetricSamplePool is a pool of metrics sample
type MetricSamplePool struct {
	pool *sync.Pool
	// telemetry
	tlmEnabled bool
}

// NewMetricSamplePool creates a new MetricSamplePool
func NewMetricSamplePool(batchSize int, isTelemetryEnabled bool) *MetricSamplePool {
	return &MetricSamplePool{
		pool: &sync.Pool{
			New: func() interface{} {
				return make(MetricSampleBatch, batchSize)
			},
		},
		// telemetry
		tlmEnabled: isTelemetryEnabled,
	}
}

// GetBatch gets a batch of metric samples from the pool
func (m *MetricSamplePool) GetBatch() MetricSampleBatch {
	if m == nil {
		return nil
	}
	if m.tlmEnabled {
		tlmMetricSamplePoolGet.Inc()
		tlmMetricSamplePool.Inc()
	}
	return m.pool.Get().(MetricSampleBatch)
}

// PutBatch puts a batch back into the pool
func (m *MetricSamplePool) PutBatch(batch MetricSampleBatch) {
	if m == nil {
		return
	}
	if m.tlmEnabled {
		tlmMetricSamplePoolPut.Inc()
		tlmMetricSamplePool.Dec()
	}
	m.pool.Put(batch[:cap(batch)])
}

// singleMetricSamplePool pools individual *MetricSample pointers for code paths
// that need a single heap-allocated MetricSample rather than a batch slot.
// The batch-level MetricSamplePool covers the hot DogStatsD parsing path;
// this pool serves auxiliary paths (e.g. late/synthetic samples, testing).
var singleMetricSamplePool = sync.Pool{
	New: func() interface{} { return &MetricSample{} },
}

// GetMetricSample returns a *MetricSample from the per-struct pool.
// The returned struct is zeroed. Call PutMetricSample to return it.
func GetMetricSample() *MetricSample {
	return singleMetricSamplePool.Get().(*MetricSample)
}

// PutMetricSample zeros s and returns it to the per-struct pool.
// The caller must not use s after this call.
func PutMetricSample(s *MetricSample) {
	// Zero the struct so the pool does not hold references to Tags slices,
	// strings, or OriginInfo that would prevent timely GC.
	*s = MetricSample{}
	singleMetricSamplePool.Put(s)
}
