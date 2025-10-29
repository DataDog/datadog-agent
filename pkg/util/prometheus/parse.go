// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

/*
Package prometheus provides utility functions to deal with prometheus endpoints
*/
package prometheus

import (
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

// MetricFamily represents a metric family that is returned by a prometheus endpoint
type MetricFamily struct {
	Name    string
	Type    string
	Samples model.Vector
}

var (
	parser = expfmt.NewTextParser(model.LegacyValidation)
)

// ParseMetricsWithFilter parses prometheus-formatted metrics from the input data, ignoring lines which contain
// text that matches the passed in filter.
func ParseMetricsWithFilter(data []byte, filter []string) ([]*MetricFamily, error) {
	// return ParseMetrics(data)
	reader := NewReader(data, filter)
	mf, err := parser.TextToMetricFamilies(reader)
	if err != nil {
		return nil, err
	}

	var metrics []*MetricFamily
	for k := range mf {
		f := mf[k]
		samples, err := expfmt.ExtractSamples(&expfmt.DecodeOptions{Timestamp: model.Now()}, f)
		if err != nil {
			return nil, err
		}
		metricFam := &MetricFamily{
			Name:    *f.Name,
			Type:    f.Type.String(),
			Samples: samples,
		}
		metrics = append(metrics, metricFam)

		// free the entry we don't need it anymore
		delete(mf, k)
	}
	return metrics, nil
}

// ParseMetrics parses prometheus-formatted metrics from the input data.
func ParseMetrics(data []byte) ([]*MetricFamily, error) {
	return ParseMetricsWithFilter(data, nil)
}
