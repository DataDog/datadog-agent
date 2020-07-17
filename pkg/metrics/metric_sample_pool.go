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

// MetricSamplePool is a pool of metrics sample
type MetricSamplePool struct {
	pool *sync.Pool
	// telemetry
	tlmEnabled bool
}

// NewMetricSamplePool creates a new MetricSamplePool
func NewMetricSamplePool(batchSize int) *MetricSamplePool {
	return &MetricSamplePool{
		pool: &sync.Pool{
			New: func() interface{} {
				return make([]MetricSample, batchSize)
			},
		},
		// telemetry
		tlmEnabled: telemetry.IsEnabled(),
	}
}

// GetBatch gets a batch of metric samples from the pool
func (m *MetricSamplePool) GetBatch() []MetricSample {
	if m == nil {
		return nil
	}
	if m.tlmEnabled {
		tlmMetricSamplePoolGet.Inc()
		tlmMetricSamplePool.Inc()
	}
	return m.pool.Get().([]MetricSample)
}

// PutBatch puts a batch back into the pool
func (m *MetricSamplePool) PutBatch(batch []MetricSample) {
	if m == nil {
		return
	}
	if m.tlmEnabled {
		tlmMetricSamplePoolPut.Inc()
		tlmMetricSamplePool.Dec()
	}
	m.pool.Put(batch[:cap(batch)])
}
