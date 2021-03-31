package dogstatsd

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// batcher batches multiple metrics before submission
// this struct is not safe for concurrent use
type batcher struct {
	samples      []metrics.MetricSample
	samplesCount int

	events        []*metrics.Event
	serviceChecks []*metrics.ServiceCheck

	// output channels
	choutSamples       chan<- []metrics.MetricSample
	choutEvents        chan<- []*metrics.Event
	choutServiceChecks chan<- []*metrics.ServiceCheck

	metricSamplePool *metrics.MetricSamplePool
}

func newBatcher(agg *aggregator.BufferedAggregator) *batcher {
	s, e, sc := agg.GetBufferedChannels()
	return &batcher{
		samples:            agg.MetricSamplePool.GetBatch(),
		metricSamplePool:   agg.MetricSamplePool,
		choutSamples:       s,
		choutEvents:        e,
		choutServiceChecks: sc,
	}
}

func (b *batcher) appendSample(sample metrics.MetricSample) {
	if b.samplesCount == len(b.samples) {
		b.flushSamples()
	}
	b.samples[b.samplesCount] = sample
	b.samplesCount++
}

func (b *batcher) appendEvent(event *metrics.Event) {
	b.events = append(b.events, event)
}

func (b *batcher) appendServiceCheck(serviceCheck *metrics.ServiceCheck) {
	b.serviceChecks = append(b.serviceChecks, serviceCheck)
}

func (b *batcher) flushSamples() {
	if b.samplesCount > 0 {
		t1 := time.Now()
		b.choutSamples <- b.samples[:b.samplesCount]
		t2 := time.Now()
		tlmChannel.Observe(float64(t2.Sub(t1).Nanoseconds()), "metrics")

		b.samplesCount = 0
		b.samples = b.metricSamplePool.GetBatch()
	}
}

// flush pushes all batched metrics to the aggregator.
func (b *batcher) flush() {
	b.flushSamples()
	if len(b.events) > 0 {
		t1 := time.Now()
		b.choutEvents <- b.events
		t2 := time.Now()
		tlmChannel.Observe(float64(t2.Sub(t1).Nanoseconds()), "events")

		b.events = []*metrics.Event{}
	}
	if len(b.serviceChecks) > 0 {
		t1 := time.Now()
		b.choutServiceChecks <- b.serviceChecks
		t2 := time.Now()
		tlmChannel.Observe(float64(t2.Sub(t1).Nanoseconds()), "service_checks")

		b.serviceChecks = []*metrics.ServiceCheck{}
	}
}
