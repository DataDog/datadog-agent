// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"fmt"
	"math"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ContextMetrics stores all the metrics by context key
type ContextMetrics map[ckey.ContextKey]Metric

// MakeContextMetrics returns a new ContextMetrics
func MakeContextMetrics() ContextMetrics {
	return ContextMetrics(make(map[ckey.ContextKey]Metric))
}

// AddSampleTelemetry counts number of new metrics added.
type AddSampleTelemetry struct {
	Total     telemetry.SimpleCounter
	Stateful  telemetry.SimpleCounter
	Stateless telemetry.SimpleCounter
}

// Inc should be called once for each new metric added to the map.
//
// isStateful should be the value returned by isStateful method for the new metric.
func (a *AddSampleTelemetry) Inc(isStateful bool) {
	a.Total.Inc()
	if isStateful {
		a.Stateful.Inc()
	} else {
		a.Stateless.Inc()
	}
}

// AddSample add a sample to the current ContextMetrics and initialize a new metrics if needed.
func (m ContextMetrics) AddSample(contextKey ckey.ContextKey, sample *MetricSample, timestamp float64, interval int64, t *AddSampleTelemetry, config pkgconfigmodel.Config) error {
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
			m[contextKey] = NewHistogram(interval, config) // default histogram configuration (no call to `configure`) for now
		case HistorateType:
			m[contextKey] = NewHistorate(interval, config) // internal histogram has the configuration for now
		case SetType:
			m[contextKey] = NewSet()
		case CounterType:
			m[contextKey] = NewCounter(interval)
		case GaugeWithTimestampType:
			m[contextKey] = NewMetricWithTimestamp(APIGaugeType)
		case CountWithTimestampType:
			m[contextKey] = NewMetricWithTimestamp(APICountType)
		default:
			err := fmt.Errorf("unknown sample metric type: %v", sample.Mtype)
			log.Error(err)
			return err
		}
		if t != nil {
			t.Inc(m[contextKey].isStateful())
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
		series = flushToSeries(
			contextKey,
			metric,
			timestamp,
			series,
			errors)
	}

	return series, errors
}

func flushToSeries(
	contextKey ckey.ContextKey,
	metric Metric,
	bucketTimestamp float64,
	series []*Serie,
	errors map[ckey.ContextKey]error,
) []*Serie {
	metricSeries, err := metric.flush(bucketTimestamp)

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
	return series
}

// aggregateContextMetricsByContextKey orders all Metric instances by context key,
// representing the result as calls to the given callbacks.  The `callback` parameter
// is called with each Metric in term, while `contextKeyChanged` is called after the
// last Metric with each context key is processed. The last argument of the callback is the index
// of the contextMetrics in contextMetricsCollection.
//
//	For example:
//	   callback(key1, metric1, 0)
//	   callback(key1, metric2, 1)
//	   callback(key1, metric3, 2)
//	   contextKeyChanged()
//	   callback(key2, metric4, 0)
//	   contextKeyChanged()
//	   callback(key3, metric5, 0)
//	   callback(key3, metric6, 1)
//	   contextKeyChanged()
func aggregateContextMetricsByContextKey(
	contextMetricsCollection []ContextMetrics,
	callback func(ckey.ContextKey, Metric, int),
	contextKeyChanged func(),
) {
	for i := 0; i < len(contextMetricsCollection); i++ {
		for contextKey, metrics := range contextMetricsCollection[i] {
			callback(contextKey, metrics, i)

			// Find `contextKey` in the remaining contextMetrics
			for j := i + 1; j < len(contextMetricsCollection); j++ {
				contextMetrics := contextMetricsCollection[j]
				if m, found := contextMetrics[contextKey]; found {
					callback(contextKey, m, j)
					delete(contextMetrics, contextKey)
				}
			}
			contextKeyChanged()
		}
	}
}
