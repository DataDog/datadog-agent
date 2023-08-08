// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"github.com/DataDog/datadog-agent/pkg/metrics/model"
)

// APIMetricType represents an API metric type
type APIMetricType = model.APIMetricType

// Enumeration of the existing API metric types
const (
	APIGaugeType = model.APIGaugeType
	APIRateType  = model.APIRateType
	APICountType = model.APICountType
)

// Metric is the interface of all metric types
type Metric interface {
	addSample(sample *MetricSample, timestamp float64)
	flush(timestamp float64) ([]*Serie, error)
	// isStateful() indicates that metric preserves information between flushes, which is
	// required for correct operation (e.g. monotonic count keeps previous value).
	isStateful() bool
}

// NoSerieError is the error returned by a metric when not enough samples have been
// submitted to generate a serie
type NoSerieError struct{}

func (e NoSerieError) Error() string {
	return "Not enough samples to generate points"
}
