package dogstatsd

import "github.com/DataDog/datadog-agent/pkg/metrics"

// MetricSample is a metric sample originating from DogStatsD
// Structuraly, this is similar to metrics.MetricSample with []byte slices
// instead of strings. Under the hood those []byte slices are pointing to
// memory allocated in the packet they were received from.
type MetricSample struct {
	Name       []byte
	Value      float64
	SetValue   []byte
	MetricType metrics.MetricType
	Tags       [][]byte
	Hostname   []byte
	SampleRate float64
	Timestamp  float64
}
