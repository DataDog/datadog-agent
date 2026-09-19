// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	metricspb "github.com/DataDog/agent-payload/v5/gogen"

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
// Agent submit implementation is /pkg/metrics/series.go.
type MetricSeriesV1 struct {
	collectedTime  time.Time
	Metric         string                      `json:"metric"`
	Type           DatadogMetricType           `json:"type"`
	Interval       uint32                      `json:"interval,omitempty"`
	Points         [][2]interface{}            `json:"points"`
	Tags           []string                    `json:"tags,omitempty"`
	Host           string                      `json:"host,omitempty"`
	SourceTypeName string                      `json:"source_type_name,omitempty"`
	Unit           string                      `json:"unit,omitempty"`
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

	inflated, err := inflate(payload.Data, payload.Encoding)
	if err != nil {
		return nil, err
	}
	header := MetricSeriesV1Header{Series: []*MetricSeriesV1{}}
	err = json.Unmarshal(inflated, &header)
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

// ParseMetricSeriesV1 decodes a /api/v1/series JSON payload.
func ParseMetricSeriesV1(payload api.Payload) ([]*MetricSeries, error) {
	v1Series, err := ParseV1MetricSeries(payload)
	if err != nil {
		return nil, err
	}

	series := make([]*MetricSeries, 0, len(v1Series))
	for _, v1 := range v1Series {
		ms := metricspb.MetricPayload_MetricSeries{
			Metric:         v1.Metric,
			Tags:           v1.Tags,
			Type:           v1MetricType(v1.Type),
			Unit:           v1.Unit,
			Interval:       int64(v1.Interval),
			SourceTypeName: v1.SourceTypeName,
		}
		// v1 carries host and device in dedicated fields; v2 carries them as resources, host first.
		if v1.Host != "" {
			ms.Resources = append(ms.Resources, &metricspb.MetricPayload_Resource{Type: "host", Name: v1.Host})
		}
		if v1.Device != "" {
			ms.Resources = append(ms.Resources, &metricspb.MetricPayload_Resource{Type: "device", Name: v1.Device})
		}
		if origin := v1.Metadata.Origin; origin != (DatadogMetricOriginMetadata{}) {
			ms.Metadata = &metricspb.Metadata{
				Origin: &metricspb.Origin{
					OriginProduct:  origin.Product,
					OriginCategory: origin.Category,
					OriginService:  origin.Service,
				},
			}
		}
		for _, point := range v1.Points {
			timestamp, value, err := v1MetricPoint(point)
			if err != nil {
				return nil, fmt.Errorf("v1 series %q: %w", v1.Metric, err)
			}
			ms.Points = append(ms.Points, &metricspb.MetricPayload_MetricPoint{Timestamp: timestamp, Value: value})
		}

		series = append(series, &MetricSeries{
			MetricPayload_MetricSeries: ms,
			collectedTime:              v1.collectedTime,
		})
	}

	return series, nil
}

func v1MetricType(metricType DatadogMetricType) metricspb.MetricPayload_MetricType {
	switch metricType {
	case Count:
		return metricspb.MetricPayload_COUNT
	case Rate:
		return metricspb.MetricPayload_RATE
	case Gauge:
		return metricspb.MetricPayload_GAUGE
	default:
		return metricspb.MetricPayload_UNSPECIFIED
	}
}

func v1MetricPoint(point [2]interface{}) (int64, float64, error) {
	timestamp, ok := point[0].(float64)
	if !ok {
		return 0, 0, fmt.Errorf("point timestamp %v is not a number", point[0])
	}
	value, ok := point[1].(float64)
	if !ok {
		return 0, 0, fmt.Errorf("point value %v is not a number", point[1])
	}
	return int64(timestamp), value, nil
}

// NewMetricAggregatorV1 returns a MetricAggregator wired to the V1 series parser.
func NewMetricAggregatorV1() MetricAggregator {
	return MetricAggregator{
		Aggregator: newAggregator(ParseMetricSeriesV1),
	}
}
