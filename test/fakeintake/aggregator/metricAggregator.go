// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"time"

	metricspb "github.com/DataDog/agent-payload/v5/gogen"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

type MetricSeries struct {
	// embed proto Metric Series struct
	metricspb.MetricPayload_MetricSeries

	collectedTime time.Time
}

func (mp *MetricSeries) name() string {
	return mp.Metric
}

// GetTags return the tags from a payload
func (mp *MetricSeries) GetTags() []string {
	return mp.Tags
}

// GetCollectedTime return the time when the payload has been collected by the fakeintake server
func (mp *MetricSeries) GetCollectedTime() time.Time {
	return mp.collectedTime
}

// ParseMetricSeries return the parsed metrics from payload
func ParseMetricSeries(payload api.Payload) (metrics []*MetricSeries, err error) {
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
		metrics = append(metrics, &MetricSeries{MetricPayload_MetricSeries: *serie, collectedTime: payload.Timestamp})
	}

	return metrics, err
}

type MetricAggregator struct {
	Aggregator[*MetricSeries]
}

func NewMetricAggregator() MetricAggregator {
	return MetricAggregator{
		Aggregator: newAggregator(ParseMetricSeries),
	}
}
