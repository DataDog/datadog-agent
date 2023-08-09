// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	metricspb "github.com/DataDog/agent-payload/v5/gogen"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// MetricSeries exported type should have comment or be unexported
type MetricSeries struct {
	// embed proto Metric Series struct
	metricspb.MetricPayload_MetricSeries
}

func (mp *MetricSeries) name() string {
	return mp.Metric
}

// GetTags exported method should have comment or be unexported
func (mp *MetricSeries) GetTags() []string {
	return mp.Tags
}

func parseMetricSeries(payload api.Payload) (metrics []*MetricSeries, err error) {
	enflated, err := enflate(payload.Data, payload.Encoding)
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

// MetricAggregator exported type should have comment or be unexported
type MetricAggregator struct {
	Aggregator[*MetricSeries]
}

// NewMetricAggregator exported function should have comment or be unexported
func NewMetricAggregator() MetricAggregator {
	return MetricAggregator{
		Aggregator: newAggregator(parseMetricSeries),
	}
}
