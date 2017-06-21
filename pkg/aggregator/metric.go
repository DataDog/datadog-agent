package aggregator

import (
	"fmt"
)

// Point represents a metric value at a specific time
type Point struct {
	Ts    int64
	Value float64
}

// MarshalJSON return a Point as an array of value (to be compatible with v1 API)
// FIXME(maxime): to be removed when v2 endpoints are available
func (p *Point) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("[%v, %v]", p.Ts, p.Value)), nil
}

// Serie holds a timeseries (w/ json serialization to DD API format)
type Serie struct {
	Name           string        `json:"metric"`
	Points         []Point       `json:"points"`
	Tags           []string      `json:"tags"`
	Host           string        `json:"host"`
	Device         string        `json:"device,omitempty"` // FIXME(olivier): remove as soon as the v1 API can handle `device` as a regular tag
	MType          APIMetricType `json:"type"`
	Interval       int64         `json:"interval"`
	SourceTypeName string        `json:"source_type_name,omitempty"`
	contextKey     string
	nameSuffix     string
}

// APIMetricType represents an API metric type
type APIMetricType int

// Enumeration of the existing API metric types
const (
	APIGaugeType APIMetricType = iota
	APIRateType
	APICountType
)

// String returns a string representation of APIMetricType
func (a APIMetricType) String() string {
	switch a {
	case APIGaugeType:
		return "gauge"
	case APIRateType:
		return "rate"
	case APICountType:
		return "count"
	default:
		return ""
	}
}

// MarshalText implements the encoding.TextMarshal interface to marshal
// an APIMetricType to a serialized byte slice
func (a APIMetricType) MarshalText() ([]byte, error) {
	str := a.String()
	if str == "" {
		return []byte{}, fmt.Errorf("Can't marshal unknown metric type %d", a)
	}

	return []byte(str), nil
}

// Metric is the interface of all metric types
type Metric interface {
	addSample(sample *MetricSample, timestamp int64)
	flush(timestamp int64) ([]*Serie, error)
}

// NoSerieError is the error returned by a metric when not enough samples have been
// submitted to generate a serie
type NoSerieError struct{}

func (e NoSerieError) Error() string {
	return "Not enough samples to generate points"
}
