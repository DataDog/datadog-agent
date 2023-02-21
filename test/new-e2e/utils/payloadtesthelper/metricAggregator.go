// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package payloadtesthelper

import (
	metricspb "github.com/DataDog/agent-payload/v5/gogen"
)

type MetricSeries struct {
	// embed proto Metric Series struct
	metricspb.MetricPayload_MetricSeries
}

func (mp *MetricSeries) name() string {
	return mp.Metric
}

func (mp *MetricSeries) tags() []string {
	return mp.Tags
}

func parseMetricSeries(data []byte) (metrics []*MetricSeries, err error) {
	enflated, err := enflate(data)
	if err != nil {
		return nil, err
	}
	metricsPayload := new(metricspb.MetricPayload)
	err = metricsPayload.Unmarshal(enflated)
	if err != nil {
		return nil, err
	}

	metrics = []*MetricSeries{}
	for _, serie := range metricsPayload.Series {
		metrics = append(metrics, &MetricSeries{MetricPayload_MetricSeries: *serie})
	}

	return metrics, err
}

type MetricAggregator struct {
	Aggregator[*MetricSeries]
}

func NewMetricAggregator() MetricAggregator {
	return MetricAggregator{
		Aggregator: newAggregator(parseMetricSeries),
	}
}
