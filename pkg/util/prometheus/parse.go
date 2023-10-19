// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

/*
Package prometheus provides utility functions to deal with prometheus endpoints
*/
package prometheus

import (
	"bytes"

	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

// MetricFamily represents a metric family that is returned by a prometheus endpoint
type MetricFamily struct {
	Name    string
	Type    string
	Samples model.Vector
}

// ParseMetrics parses prometheus-formatted metrics from the input data.
func ParseMetrics(data []byte) ([]*MetricFamily, error) {
	// the prometheus TextParser does not support windows line separators, so we need to explicitly remove them
	data = bytes.Replace(data, []byte("\r"), []byte(""), -1)

	reader := bytes.NewReader(data)
	var parser expfmt.TextParser
	mf, err := parser.TextToMetricFamilies(reader)
	if err != nil {
		return nil, err
	}

	var metrics []*MetricFamily
	for _, family := range mf {
		samples, err := expfmt.ExtractSamples(&expfmt.DecodeOptions{Timestamp: model.Now()}, family)
		if err != nil {
			return nil, err
		}
		metricFam := &MetricFamily{
			Name:    *family.Name,
			Type:    family.Type.String(),
			Samples: samples,
		}
		metrics = append(metrics, metricFam)
	}
	return metrics, nil
}
