package dogstatsd

import (
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// batcher batchs multiple metrics before submission
// this struct is not safe for concurent use
type batcher struct {
	samples       []metrics.MetricSample
	samplesCount  int
	events        []*metrics.Event
	serviceChecks []*metrics.ServiceCheck

	samplePool      *metrics.MetricSamplePool
	sampleOut       chan<- []metrics.MetricSample
	eventOut        chan<- []*metrics.Event
	serviceCheckOut chan<- []*metrics.ServiceCheck
}

func newBatcher(samplePool *metrics.MetricSamplePool, sampleOut chan<- []metrics.MetricSample, eventOut chan<- []*metrics.Event, serviceCheckOut chan<- []*metrics.ServiceCheck) *batcher {
	return &batcher{
		samples:         samplePool.GetBatch(),
		samplePool:      samplePool,
		sampleOut:       sampleOut,
		eventOut:        eventOut,
		serviceCheckOut: serviceCheckOut,
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
		b.sampleOut <- b.samples[:b.samplesCount]
		b.samplesCount = 0
		b.samples = b.samplePool.GetBatch()
	}
}

func (b *batcher) flush() {
	b.flushSamples()
	if len(b.events) > 0 {
		b.eventOut <- b.events
		b.events = []*metrics.Event{}
	}
	if len(b.serviceChecks) > 0 {
		b.serviceCheckOut <- b.serviceChecks
		b.serviceChecks = []*metrics.ServiceCheck{}
	}
}
