package aggregator

// MetricType is the representation of a dogstatsd metric type
type MetricType string

// metric type constants
const (
	GaugeType   MetricType = "g"
	RateType    MetricType = "rate"
	CounterType MetricType = "c"
)

// MetricSample represents a raw metric sample
type MetricSample struct {
	Name       string
	Value      float64
	Mtype      MetricType
	Tags       *[]string
	SampleRate float64
	Timestamp  int64
}
