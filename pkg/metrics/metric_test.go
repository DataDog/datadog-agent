// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

package metrics

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

type DummyMetric struct {
	Type APIMetricType `json:"type"`
}

func TestAPIMetricTypeMarshal(t *testing.T) {
	for _, tc := range []struct {
		In  *DummyMetric
		Out string
	}{
		{
			&DummyMetric{Type: APIGaugeType},
			`{"type":"gauge"}`,
		},
		{
			&DummyMetric{Type: APICountType},
			`{"type":"count"}`,
		},
		{
			&DummyMetric{Type: APIRateType},
			`{"type":"rate"}`,
		},
	} {
		t.Run(tc.Out, func(t *testing.T) {
			out, err := json.Marshal(tc.In)
			assert.NoError(t, err)
			assert.Equal(t, tc.Out, string(out))

			back := &DummyMetric{}
			err = json.Unmarshal(out, back)
			assert.NoError(t, err)
			assert.Equal(t, tc.In, back)
		})
	}
}

func TestAPIMetricTypeString(t *testing.T) {
	tests := []struct {
		metricType APIMetricType
		expected   string
	}{
		{APIGaugeType, "gauge"},
		{APIRateType, "rate"},
		{APICountType, "count"},
		{APIMetricType(999), ""},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.metricType.String())
		})
	}
}

func TestAPIMetricTypeMarshalTextUnknown(t *testing.T) {
	unknownType := APIMetricType(999)
	_, err := unknownType.MarshalText()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Can't marshal unknown metric type")
}

func TestNoSerieError(t *testing.T) {
	err := NoSerieError{}
	assert.Equal(t, "Not enough samples to generate points", err.Error())
}
