// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package telemetry defines portable, typed representations of captured Agent output.
package telemetry

import (
	"errors"
	"math"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// MetricSample preserves fields needed by both Agent metric protocols. Marshaling
// metrics.Serie directly loses Source and rounds fractional relative timestamps.
type MetricSample struct {
	Series []MetricSeries `json:"series"`
}

// MetricSeries is the capture allowlist, independent of metrics.Serie's wire JSON.
type MetricSeries struct {
	Name     string                `json:"metric"`
	Host     string                `json:"host"`
	Device   string                `json:"device,omitempty"`
	Tags     []string              `json:"tags"`
	Type     string                `json:"type"`
	Interval int64                 `json:"interval"`
	Source   *metrics.MetricSource `json:"source"`
	Points   []MetricPoint         `json:"points"`
}

// MetricPoint retains a fractional timestamp relative to capture start.
type MetricPoint struct {
	Timestamp float64 `json:"timestamp"`
	Value     float64 `json:"value"`
}

// NewMetricSample takes an owned copy of the sanitized Agent series.
func NewMetricSample(series []*metrics.Serie) (*MetricSample, error) {
	sample := &MetricSample{Series: make([]MetricSeries, 0, len(series))}
	for _, serie := range series {
		if serie == nil {
			return nil, errors.New("metric sample contains a nil series")
		}
		source := serie.Source
		value := MetricSeries{Name: serie.Name, Host: serie.Host, Device: serie.Device, Tags: slices.Clone(serie.Tags.UnsafeToReadOnlySliceString()), Type: serie.MType.String(), Interval: serie.Interval, Source: &source}
		for _, point := range serie.Points {
			value.Points = append(value.Points, MetricPoint{Timestamp: point.Ts, Value: point.Value})
		}
		sample.Series = append(sample.Series, value)
	}
	if _, err := sample.AgentSeries(); err != nil {
		return nil, err
	}
	return sample, nil
}

// AgentSeries validates the envelope and reconstructs fresh Agent series.
func (s *MetricSample) AgentSeries() ([]*metrics.Serie, error) {
	if s == nil || len(s.Series) == 0 {
		return nil, errors.New("metric sample contains no series")
	}
	result := make([]*metrics.Serie, 0, len(s.Series))
	for _, serie := range s.Series {
		if serie.Name == "" || serie.Host == "" || len(serie.Points) == 0 || serie.Source == nil || serie.Interval < 0 {
			return nil, errors.New("metric series lacks its name, host, source, points, or valid interval")
		}
		if *serie.Source != metrics.MetricSourceUnknown && serie.Source.String() == "<unknown>" {
			return nil, errors.New("metric series has an unsupported source")
		}
		var mtype metrics.APIMetricType
		switch serie.Type {
		case "gauge":
			mtype = metrics.APIGaugeType
		case "rate":
			mtype = metrics.APIRateType
		case "count":
			mtype = metrics.APICountType
		default:
			return nil, errors.New("metric series has an unsupported type")
		}
		value := &metrics.Serie{Name: serie.Name, Host: serie.Host, Device: serie.Device, Tags: tagset.CompositeTagsFromSlice(slices.Clone(serie.Tags)), MType: mtype, Interval: serie.Interval, Source: *serie.Source}
		for _, point := range serie.Points {
			if math.IsNaN(point.Timestamp) || math.IsInf(point.Timestamp, 0) || math.IsNaN(point.Value) || math.IsInf(point.Value, 0) {
				return nil, errors.New("metric point must be finite")
			}
			value.Points = append(value.Points, metrics.Point{Ts: point.Timestamp, Value: point.Value})
		}
		result = append(result, value)
	}
	return result, nil
}
