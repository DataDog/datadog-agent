// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"fmt"
)

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

// UnmarshalText is a custom unmarshaller for APIMetricType (used for testing)
func (a *APIMetricType) UnmarshalText(buf []byte) error {
	switch string(buf) {
	case "gauge":
		*a = APIGaugeType
	case "rate":
		*a = APIRateType
	case "count":
		*a = APICountType
	}
	return nil
}

// Metric is the interface of all metric types
type Metric interface {
	addSample(sample *MetricSample, timestamp float64)
	flush(timestamp float64) ([]*Serie, error)
}

// NoSerieError is the error returned by a metric when not enough samples have been
// submitted to generate a serie
type NoSerieError struct{}

func (e NoSerieError) Error() string {
	return "Not enough samples to generate points"
}
