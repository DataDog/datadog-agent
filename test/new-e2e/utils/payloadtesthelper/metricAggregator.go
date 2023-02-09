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
	metricsByName map[string][]*MetricSeries
}

func NewMetricAggregator() MetricAggregator {
	return MetricAggregator{
		metricsByName: map[string][]*MetricSeries{},
	}
}

func (agg *MetricAggregator) UnmarshallPayloads(body []byte) error {
	metricsByName, err := unmarshallPayloads(body, parseMetricSeries)
	if err != nil {
		return err
	}
	agg.metricsByName = metricsByName
	return nil
}

func (agg *MetricAggregator) ContainsMetricName(name string) bool {
	_, found := agg.metricsByName[name]
	return found
}

func (agg *MetricAggregator) ContainsMetricNameAndTags(name string, tags []string) bool {
	series, found := agg.metricsByName[name]
	if !found {
		return false
	}

	for _, serie := range series {
		if areTagsSubsetOfOtherTags(tags, serie.Tags) {
			return true
		}
	}

	return false
}
