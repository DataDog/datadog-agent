// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scraper

import (
	"math"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util/prometheus"
)

// SampleData holds a processed sample with its computed tags and hostname.
type SampleData struct {
	Sample   *prometheus.Sample
	Tags     []string
	Hostname string
}

// TransformerFunc transforms a metric family's samples and submits them via sender.
type TransformerFunc func(metricName string, samples []SampleData, sndr sender.Sender, flushFirstValue bool)

// shouldSkip returns true if the sample value is NaN or Inf and should be ignored.
func shouldSkip(value float64) bool {
	return math.IsNaN(value) || math.IsInf(value, 0)
}

// newGaugeTransformer returns a TransformerFunc that submits each sample as a Gauge.
func newGaugeTransformer() TransformerFunc {
	return func(metricName string, samples []SampleData, sndr sender.Sender, _ bool) {
		for i := range samples {
			sd := &samples[i]
			if shouldSkip(sd.Sample.Value) {
				continue
			}
			sndr.Gauge(metricName, sd.Sample.Value, sd.Hostname, sd.Tags)
		}
	}
}

// newCounterTransformer returns a TransformerFunc that submits each sample as
// a MonotonicCount with a ".count" suffix.
func newCounterTransformer() TransformerFunc {
	return func(metricName string, samples []SampleData, sndr sender.Sender, _ bool) {
		for i := range samples {
			sd := &samples[i]
			if shouldSkip(sd.Sample.Value) {
				continue
			}
			sndr.MonotonicCount(metricName+".count", sd.Sample.Value, sd.Hostname, sd.Tags)
		}
	}
}

// newRateTransformer returns a TransformerFunc that submits each sample as a Rate.
func newRateTransformer() TransformerFunc {
	return func(metricName string, samples []SampleData, sndr sender.Sender, _ bool) {
		for i := range samples {
			sd := &samples[i]
			if shouldSkip(sd.Sample.Value) {
				continue
			}
			sndr.Rate(metricName, sd.Sample.Value, sd.Hostname, sd.Tags)
		}
	}
}

// newCounterGaugeTransformer returns a TransformerFunc that submits each sample
// as both a Gauge (with ".total" suffix) and a MonotonicCount (with ".count" suffix).
func newCounterGaugeTransformer() TransformerFunc {
	return func(metricName string, samples []SampleData, sndr sender.Sender, _ bool) {
		for i := range samples {
			sd := &samples[i]
			if shouldSkip(sd.Sample.Value) {
				continue
			}
			sndr.Gauge(metricName+".total", sd.Sample.Value, sd.Hostname, sd.Tags)
			sndr.MonotonicCount(metricName+".count", sd.Sample.Value, sd.Hostname, sd.Tags)
		}
	}
}

// newSummaryTransformer returns a TransformerFunc that handles summary-type metrics.
// _sum samples are submitted as MonotonicCount, _count samples as MonotonicCount,
// and quantile samples as Gauge.
func newSummaryTransformer(_ bool) TransformerFunc {
	return func(metricName string, samples []SampleData, sndr sender.Sender, _ bool) {
		for i := range samples {
			sd := &samples[i]
			if shouldSkip(sd.Sample.Value) {
				continue
			}
			name := sd.Sample.Metric["__name__"]
			switch {
			case strings.HasSuffix(name, "_sum"):
				sndr.MonotonicCount(metricName+".sum", sd.Sample.Value, sd.Hostname, sd.Tags)
			case strings.HasSuffix(name, "_count"):
				sndr.MonotonicCount(metricName+".count", sd.Sample.Value, sd.Hostname, sd.Tags)
			default:
				sndr.Gauge(metricName+".quantile", sd.Sample.Value, sd.Hostname, sd.Tags)
			}
		}
	}
}
