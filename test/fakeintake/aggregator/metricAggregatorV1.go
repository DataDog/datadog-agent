// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"encoding/json"
	metricspb "github.com/DataDog/agent-payload/v5/gogen"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"time"
)

type DatadogPoint struct {
	I64 int64
	F64 float64
}

type DatadogMetricOriginMetadata struct {
	Product  uint32
	Category uint32
	Service  uint32
}

type MetricSeriesV1 struct {
	collectedTime     time.Time
	Metric            string         `json:"metric"`
	Interval          uint32         `json:"interval,omitempty",`
	DatadogMetricType uint8          `json:"type"`
	Points            []DatadogPoint `json:"points"`
	Tags              []string       `json:"tags,omitempty"`
	Host              string         `json:"host,omitempty"`
	SourceTypeName    string         `json:"source_type_name,omitempty"`
	Device            string         `json:"device,omitempty"`
}

func (mp *MetricSeriesV1) name() string {
	return mp.Metric
}

// GetTags return the tags from a payload
func (mp *MetricSeriesV1) GetTags() []string {
	return mp.Tags
}

// GetCollectedTime return the time when the payload has been collected by the fakeintake server
func (mp *MetricSeriesV1) GetCollectedTime() time.Time {
	return mp.collectedTime
}

// ParseMetricSeries return the parsed metrics from payload
func ParseV1MetricSeries(payload api.Payload) (metrics []*MetricSeriesV1, err error) {
	enflated, err := enflate(payload.Data, payload.Encoding)
	if err != nil {
		return nil, err
	}
	metricsPayload := new(metricspb.MetricPayload)
	err = metricsPayload.Unmarshal(enflated)
	if err != nil {
		return nil, err
	}

	metrics = []*MetricSeriesV1{}
	err = json.Unmarshal(enflated, &metrics)
	if err != nil {
		return nil, err
	}
	for _, l := range metrics {
		l.collectedTime = payload.Timestamp
	}
	return metrics, err

}

type MetricAggregatorV1 struct {
	Aggregator[*MetricSeriesV1]
}

func NewMetricAggregatorV1() MetricAggregatorV1 {
	return MetricAggregatorV1{
		Aggregator: newAggregator(ParseV1MetricSeries),
	}
}
