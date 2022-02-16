package flowaggregator

import (
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"sync"
	"time"
)

const DefaultFlushInterval = 15 * time.Second // flush interval

// BufferedAggregator aggregates metrics in buckets for dogstatsd Metrics
type BufferedAggregator struct {
	bufferedMetricIn       chan []metrics.MetricSample
	bufferedMetricInWithTs chan []metrics.MetricSample
	bufferedServiceCheckIn chan []*metrics.ServiceCheck
	bufferedEventIn        chan []*metrics.Event

	metricIn      chan *metrics.MetricSample
	flushInterval time.Duration
	mu            sync.Mutex // to protect the checkSamplers field
}
