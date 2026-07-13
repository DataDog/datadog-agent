// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"math"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

const (
	dogstatsdClientBytesDropped = "datadog.dogstatsd.client.bytes_dropped"
	dogstatsdClientBytesSent    = "datadog.dogstatsd.client.bytes_sent"
	droppedBytesWarnThreshold   = 0.01
)

// hostByteStats accumulates dropped and sent byte counts for one host per flush.
type hostByteStats struct {
	dropped float64
	sent    float64
}

// droppedMetricsDetector tracks bytes_dropped and bytes_sent per host across a
// single flush cycle and logs a warning for any host exceeding the drop threshold.
type droppedMetricsDetector struct {
	hosts map[string]*hostByteStats
}

func newDroppedMetricsDetector() *droppedMetricsDetector {
	return &droppedMetricsDetector{hosts: make(map[string]*hostByteStats)}
}

func (d *droppedMetricsDetector) observe(serie *metrics.Serie) {
	if serie.Name != dogstatsdClientBytesDropped && serie.Name != dogstatsdClientBytesSent {
		return
	}
	// DogStatsD count submissions are flushed as rate series. Requiring that
	// type prevents a custom gauge using one of these reserved names from
	// affecting the detector.
	if serie.MType != metrics.APIRateType {
		return
	}
	var total float64
	for _, p := range serie.Points {
		// Client byte telemetry is non-negative. Ignore malformed series rather
		// than allowing a negative or non-finite value to create a bogus ratio.
		if p.Value < 0 || math.IsNaN(p.Value) || math.IsInf(p.Value, 0) {
			return
		}
		total += p.Value
	}
	stats := d.hosts[serie.Host]
	if stats == nil {
		stats = &hostByteStats{}
		d.hosts[serie.Host] = stats
	}
	if serie.Name == dogstatsdClientBytesDropped {
		stats.dropped += total
	} else {
		stats.sent += total
	}
}

// violations returns the drop ratio for every host that exceeds the threshold.
// Separated from logging so the detection logic can be tested directly.
func (d *droppedMetricsDetector) violations() map[string]float64 {
	result := make(map[string]float64)
	for host, stats := range d.hosts {
		total := stats.dropped + stats.sent
		if total == 0 {
			continue
		}
		if ratio := stats.dropped / total; ratio > droppedBytesWarnThreshold {
			result[host] = ratio
		}
	}
	return result
}

func (d *droppedMetricsDetector) logWarnings(logger log.Component) {
	for host, ratio := range d.violations() {
		stats := d.hosts[host]
		logger.Warnf("DogStatsD client telemetry for host %q reports %.2f%% dropped metric bytes (dropped=%.0f sent=%.0f)",
			host, ratio*100, stats.dropped, stats.sent)
	}
}

// observingSerieSink wraps a SerieSink, forwarding all Append calls while also
// feeding each serie to a droppedMetricsDetector.
type observingSerieSink struct {
	inner    metrics.SerieSink
	detector *droppedMetricsDetector
}

func (s *observingSerieSink) Append(serie *metrics.Serie) {
	s.detector.observe(serie)
	s.inner.Append(serie)
}
