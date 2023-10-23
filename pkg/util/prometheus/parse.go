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

const (
	// TypeLabel is the special tag which signifies the type of the metric collected from Prometheus
	TypeLabel = "__type__"
)

// ParseMetrics parses prometheus-formatted metrics from the input data.
func ParseMetrics(data []byte) (model.Vector, error) {
	// the prometheus TextParser does not support windows line separators, so we need to explicitly remove them
	data = bytes.Replace(data, []byte("\r"), []byte(""), -1)

	reader := bytes.NewReader(data)
	var parser expfmt.TextParser
	mf, err := parser.TextToMetricFamilies(reader)
	if err != nil {
		return nil, err
	}

	var metrics model.Vector
	for _, family := range mf {
		samples, err := expfmt.ExtractSamples(&expfmt.DecodeOptions{Timestamp: model.Now()}, family)
		if err != nil {
			return nil, err
		}
		for i := range samples {
			// explicitly set the metric type as a label, as it will help when handling the metric
			samples[i].Metric[TypeLabel] = model.LabelValue(family.Type.String())
		}
		metrics = append(metrics, samples...)
	}
	return metrics, nil
}
