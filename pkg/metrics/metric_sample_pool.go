package metrics

import (
	"sync"
)

var (
	// GlobalMetricSamplePool is a pool to avoid allocation while
	// manipulating MetricSample.
	GlobalMetricSamplePool = NewMetricSamplePool(32)
)

// MetricSamplePool is a pool of metrics sample
type MetricSamplePool struct {
	pool *sync.Pool
}

// NewMetricSamplePool creates a new MetricSamplePool
func NewMetricSamplePool(batchSize int) *MetricSamplePool {
	return &MetricSamplePool{
		pool: &sync.Pool{
			New: func() interface{} {
				return make([]MetricSample, batchSize)
			},
		},
	}
}

// GetBatch gets a batch of metric samples from the pool
func (m *MetricSamplePool) GetBatch() []MetricSample {
	if m == nil {
		return nil
	}
	return m.pool.Get().([]MetricSample)
}

// PutBatch puts a batch back into the pool
func (m *MetricSamplePool) PutBatch(batch []MetricSample) {
	if m == nil {
		return
	}
	m.pool.Put(batch[:cap(batch)])
}
