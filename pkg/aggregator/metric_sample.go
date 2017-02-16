package aggregator

// MetricType is the representation of an aggregator metric type
type MetricType int

// metric type constants enumeration
const (
	GaugeType MetricType = iota
	RateType
	CountType
	MonotonicCountType
	CounterType
	HistogramType
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
