// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package metrics

import (
	"fmt"
	"math"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ContextMetrics stores all the metrics by context key
type ContextMetrics map[ckey.ContextKey]Metric

// MakeContextMetrics returns a new ContextMetrics
func MakeContextMetrics() ContextMetrics {
	return ContextMetrics(make(map[ckey.ContextKey]Metric))
}

// AddSample add a sample to the current ContextMetrics and initialize a new metrics if needed.
func (m ContextMetrics) AddSample(contextKey ckey.ContextKey, sample *MetricSample, timestamp float64, interval int64) error {
	if math.IsInf(sample.Value, 0) || math.IsNaN(sample.Value) {
		return fmt.Errorf("sample with value '%v'", sample.Value)
	}
	if _, ok := m[contextKey]; !ok {
		switch sample.Mtype {
		case GaugeType:
			m[contextKey] = &Gauge{}
		case RateType:
			m[contextKey] = &Rate{}
		case CountType:
			m[contextKey] = &Count{}
		case MonotonicCountType:
			m[contextKey] = &MonotonicCount{}
		case HistogramType:
			m[contextKey] = NewHistogram(interval) // default histogram configuration (no call to `configure`) for now
		case HistorateType:
			m[contextKey] = NewHistorate(interval) // internal histogram has the configuration for now
		case SetType:
			m[contextKey] = NewSet()
		case CounterType:
			m[contextKey] = NewCounter(interval)
		default:
			err := fmt.Errorf("unknown sample metric type: %v", sample.Mtype)
			log.Error(err)
			return err
		}
	}
	m[contextKey].addSample(sample, timestamp)
	return nil
}

// Flush flushes every metrics in the ContextMetrics.
// Returns the slice of Series and a map of errors by context key.
func (m ContextMetrics) Flush(timestamp float64) ([]*Serie, map[ckey.ContextKey]error) {
	var series []*Serie
	errors := make(map[ckey.ContextKey]error)

	for contextKey, metric := range m {
		metricSeries, err := metric.flush(timestamp)

		if err == nil {
			for _, serie := range metricSeries {
				serie.ContextKey = contextKey
				series = append(series, serie)
			}
		} else {
			switch err.(type) {
			case NoSerieError:
				// this error happens in nominal conditions and shouldn't be returned
			default:
				errors[contextKey] = err
			}
		}
	}

	return series, errors
}
