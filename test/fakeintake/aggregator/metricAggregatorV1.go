// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// DatadogMetricType represent different metrics types
type DatadogMetricType string

const (
	// Gauge DatadogMetricType
	Gauge DatadogMetricType = "gauge"
	// Count DatadogMetricType
	Count DatadogMetricType = "count"
	// Rate DatadogMetricType
	Rate DatadogMetricType = "rate"
)

// MetricSeriesV1Header contains a MetricSeriesV1
type MetricSeriesV1Header struct {
	Series []*MetricSeriesV1 `json:"series"`
}

// MetricSeriesV1 contains all information of a metric in V1
// Following API specifications V1 https://docs.datadoghq.com/api/latest/metrics/#submit-metrics
// Agent submit implementation is https://github.com/DataDog/datadog-agent/blob/af712ce616266987e35e621f0c8f78cbc81716f9/pkg/metrics/series.go#L49-L63
type MetricSeriesV1 struct {
	collectedTime  time.Time
	Metric         string                      `json:"metric"`
	Type           DatadogMetricType           `json:"type"`
	Interval       uint32                      `json:"interval,omitempty"`
	Points         [][2]interface{}            `json:"points"`
	Tags           []string                    `json:"tags,omitempty"`
	Host           string                      `json:"host,omitempty"`
	SourceTypeName string                      `json:"source_type_name,omitempty"`
	Device         string                      `json:"device,omitempty"`
	Metadata       DatadogSeriesMetricMetadata `json:"metadata,omitempty"`
}

// DatadogSeriesMetricMetadata contains DatadogMetricMetadata
type DatadogSeriesMetricMetadata struct {
	Origin DatadogMetricOriginMetadata `json:"origin,omitempty"`
}

// DatadogMetricOriginMetadata informations
type DatadogMetricOriginMetadata struct {
	// OriginProduct
	Product uint32 `json:"product,omitempty"`
	// OriginCategory
	Category uint32 `json:"category,omitempty"`
	// OriginService
	Service uint32 `json:"service,omitempty"`
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

// ParseV1MetricSeries return the parsed metrics from payload
func ParseV1MetricSeries(payload api.Payload) (metrics []*MetricSeriesV1, err error) {
	if bytes.Equal(payload.Data, []byte("{}")) {
		// metrics can submit empty JSON object
		return []*MetricSeriesV1{}, nil
	}

	enflated, err := enflate(payload.Data, payload.Encoding)
	if err != nil {
		return nil, err
	}
	header := MetricSeriesV1Header{Series: []*MetricSeriesV1{}}
	err = json.Unmarshal(enflated, &header)
	if err != nil {
		return nil, err
	}
	for _, l := range header.Series {
		l.collectedTime = payload.Timestamp
	}
	return header.Series, err

}

// MetricAggregatorV1 Aggregator
type MetricAggregatorV1 struct {
	Aggregator[*MetricSeriesV1]
}
