package dogstatsd

import "github.com/DataDog/datadog-agent/pkg/metrics"

const batchSize = 32

// MetricSampleBatch is a batch of metric samples to be sent to the aggregator
type MetricSampleBatch struct {
	Samples [batchSize]MetricSample
	Count   int
}

// Add adds a sample to the batch
func (b *MetricSampleBatch) Add(sample MetricSample) {
	if b.Count < batchSize {
		b.Samples[b.Count] = sample
		b.Count++
	}
}

// IsFull returns
func (b *MetricSampleBatch) IsFull() bool {
	return b.Count >= batchSize
}

// MetricSample is a metric sample originating from DogStatsD
// Structuraly, this is similar to metrics.MetricSample with []byte slices
// instead of strings. Under the hood those []byte slices are pointing to
// memory allocated in the packet they were received from.
type MetricSample struct {
	packet     *Packet
	Name       []byte
	Value      float64
	SetValue   []byte
	MetricType metrics.MetricType
	Tags       [][]byte
	Hostname   []byte
	SampleRate float64
	Timestamp  float64
}

// Release removes one from the underlying packet reference counting
func (s *MetricSample) Release() {
	if s.packet != nil {
		s.packet.release()
	}
}

// EventBatch is a batch of events to be sent to the aggregator
type EventBatch struct {
	Events [batchSize]Event
	Count  int
}

// Add adds an event to the batch
func (b *EventBatch) Add(event Event) {
	if b.Count < batchSize {
		b.Events[b.Count] = event
		b.Count++
	}
}

func (b *EventBatch) IsFull() bool {
	return b.Count >= batchSize
}

// Event is an event originating from DogStatsD
// Structuraly, this is similar to metrics.Event with []byte slices
// instead of strings. Under the hood those []byte slices are pointing to
// memory allocated in the packet they were received from.
type Event struct {
	packet         *Packet
	Title          []byte
	Text           []byte
	Timestamp      int64
	Priority       metrics.EventPriority
	Hostname       []byte
	Tags           [][]byte
	ExtraTags      []string
	AlertType      metrics.EventAlertType
	AggregationKey []byte
	SourceTypeName []byte
}

// Release removes one from the underlying packet reference counting
func (e *Event) Release() {
	if e.packet != nil {
		e.packet.release()
	}
}

// ServiceCheckBatch is a batch of service checks to be sent to the aggregator
type ServiceCheckBatch struct {
	ServiceChecks [batchSize]ServiceCheck
	Count         int
}

// Add adds an event to the batch
func (b *ServiceCheckBatch) Add(serviceCheck ServiceCheck) {
	if b.Count < batchSize {
		b.ServiceChecks[b.Count] = serviceCheck
		b.Count++
	}
}

func (b *ServiceCheckBatch) IsFull() bool {
	return b.Count >= batchSize
}

// ServiceCheck is a service check originating from DogStatsD
// Structuraly, this is similar to metrics.ServiceCheck with []byte slices
// instead of strings. Under the hood those []byte slices are pointing to
// memory allocated in the packet they were received from.
type ServiceCheck struct {
	packet    *Packet
	Name      []byte
	Hostname  []byte
	Timestamp int64
	Status    metrics.ServiceCheckStatus
	Message   []byte
	Tags      [][]byte
	ExtraTags []string
}

// Release removes one from the underlying packet reference counting
func (sc *ServiceCheck) Release() {
	if sc.packet != nil {
		sc.packet.release()
	}
}
