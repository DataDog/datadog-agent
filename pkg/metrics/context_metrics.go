// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package metrics

import (
	"math"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	log "github.com/cihub/seelog"
)

// ContextKey is a non-cryptographic hash unique to a context
type ContextKey [16]byte

// ContextMetrics stores all the metrics by context key
type ContextMetrics map[ckey.ContextKey]Metric

// MakeContextMetrics returns a new ContextMetrics
func MakeContextMetrics() ContextMetrics {
	return ContextMetrics(make(map[ckey.ContextKey]Metric))
}

// AddSample add a sample to the current ContextMetrics and initialize a new metrics if needed.
// TODO: Pass a reference to *MetricSample instead
func (m ContextMetrics) AddSample(contextKey ckey.ContextKey, sample *MetricSample, timestamp float64, interval int64) {
	if math.IsInf(sample.Value, 0) || math.IsNaN(sample.Value) {
		log.Debug("Ignoring sample with ", sample.Value, " value on context key:", contextKey)
		return
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
			log.Error("Can't add unknown sample metric type:", sample.Mtype)
			return
		}
	}
	m[contextKey].addSample(sample, timestamp)
}

// Flush flushes every metrics in the ContextMetrics
func (m ContextMetrics) Flush(timestamp float64) []*Serie {
	var series []*Serie

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
				// this error happens in nominal conditions and shouldn't be logged
			default:
				log.Infof("An error occurred while flushing metric on context key '%s': %s", contextKey, err)
			}
		}
	}

	return series
}
