// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package prometheus

import (
	"bytes"

	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"golang.org/x/exp/maps"
)

// ParseMetrics parses prometheus-formatted metrics from the input data.
func ParseMetrics(data []byte) (model.Vector, error) {
	reader := bytes.NewReader(data)
	var parser expfmt.TextParser
	mf, err := parser.TextToMetricFamilies(reader)
	if err != nil {
		return nil, err
	}

	metrics, err := expfmt.ExtractSamples(&expfmt.DecodeOptions{Timestamp: model.Now()}, maps.Values(mf)...)
	if err != nil {
		return nil, err
	}
	return metrics, nil
}
