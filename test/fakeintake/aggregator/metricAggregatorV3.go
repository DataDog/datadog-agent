// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"bytes"
	"fmt"

	metricspb "github.com/DataDog/agent-payload/v5/gogen"
	intake_v3 "github.com/DataDog/agent-payload/v5/metrics/intake_v3"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// ParseMetricSeriesV3 decodes a /api/intake/metrics/v3/series payload (compressed protobuf)
// into []*MetricSeries — the same type used for /api/v2/series — so FilterMetrics() works
// transparently regardless of which wire format the agent uses.
//
// V3 is a column-oriented protobuf encoding defined in
// github.com/DataDog/agent-payload/v5/metrics/intake_v3. The MetricType numeric values are
// identical to MetricPayload_MetricType (COUNT=1, RATE=2, GAUGE=3), making the cast safe.
func ParseMetricSeriesV3(payload api.Payload) ([]*MetricSeries, error) {
	if len(payload.Data) == 0 || bytes.Equal(payload.Data, []byte("{}")) {
		return []*MetricSeries{}, nil
	}

	inflated, err := inflate(payload.Data, payload.Encoding)
	if err != nil {
		return nil, fmt.Errorf("v3 payload inflate: %w", err)
	}
	if len(inflated) == 0 || bytes.Equal(inflated, []byte("{}")) {
		return []*MetricSeries{}, nil
	}

	var p intake_v3.Payload
	if err := proto.Unmarshal(inflated, &p); err != nil {
		return nil, fmt.Errorf("v3 payload unmarshal: %w", err)
	}
	if p.MetricData == nil {
		return []*MetricSeries{}, nil
	}

	r := NewReader(p.MetricData)
	if err := r.Initialize(); err != nil {
		return nil, fmt.Errorf("v3 reader init: %w", err)
	}

	var series []*MetricSeries
	for r.HaveMoreMetrics() {
		if err := r.NextMetric(); err != nil {
			return nil, fmt.Errorf("v3 next metric: %w", err)
		}

		metricType := r.Type()
		if metricType == intake_v3.MetricType_Sketch {
			return nil, fmt.Errorf("unexpected sketch metric %q in V3 series payload", r.Name())
		}

		ms := &metricspb.MetricPayload_MetricSeries{
			Metric: r.Name(),
			Tags:   append([]string{}, r.Tags()...),
			// MetricType numeric values are identical between the two proto packages.
			Type: metricspb.MetricPayload_MetricType(metricType),
			Unit: r.Unit(),
		}
		for _, res := range r.Resources() {
			if res == nil {
				continue
			}
			ms.Resources = append(ms.Resources, &metricspb.MetricPayload_Resource{
				Type: res[0],
				Name: res[1],
			})
		}

		for r.HaveMorePoints() {
			if err := r.NextPoint(); err != nil {
				return nil, fmt.Errorf("v3 next point: %w", err)
			}
			ms.Points = append(ms.Points, &metricspb.MetricPayload_MetricPoint{
				Timestamp: r.Timestamp(),
				Value:     r.Value(),
			})
		}

		series = append(series, &MetricSeries{
			MetricPayload_MetricSeries: *ms,
			collectedTime:              payload.Timestamp,
		})
	}

	return series, nil
}

// NewMetricAggregatorV3 returns a MetricAggregator wired to the V3 series parser.
// The returned type is identical to NewMetricAggregator(); callers can merge results
// from both aggregators and pass them to FilterMetrics without type conversion.
func NewMetricAggregatorV3() MetricAggregator {
	return MetricAggregator{
		Aggregator: newAggregator(ParseMetricSeriesV3),
	}
}
