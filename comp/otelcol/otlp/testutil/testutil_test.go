// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package testutil

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGaugeMetrics(t *testing.T) {
	m := NewGaugeMetrics([]TestGauge{
		{
			Name: "metric1",
			DataPoints: []DataPoint{
				{
					Value:      1,
					Attributes: map[string]string{"a": "b", "c": "d", "e": "f"},
				},
			},
		},
		{
			Name: "metric2",
			DataPoints: []DataPoint{
				{
					Value:      2,
					Attributes: map[string]string{"x": "y", "z": "q", "w": "e"},
				},
				{
					Value:      3,
					Attributes: map[string]string{"w": "n"},
				},
			},
		},
	})
	all := m.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()
	require.Equal(t, all.Len(), 2)
	require.Equal(t, all.At(0).Name(), "metric1")
	require.Equal(t, all.At(0).Gauge().DataPoints().At(0).DoubleValue(), float64(1))
	require.EqualValues(t, all.At(0).Gauge().DataPoints().At(0).Attributes().AsRaw(), map[string]any{
		"a": "b", "c": "d", "e": "f",
	})
	require.Equal(t, all.At(1).Name(), "metric2")
	require.Equal(t, all.At(1).Gauge().DataPoints().At(0).DoubleValue(), float64(2))
	require.EqualValues(t, all.At(1).Gauge().DataPoints().At(0).Attributes().AsRaw(), map[string]any{
		"x": "y", "z": "q", "w": "e",
	})
	require.Equal(t, all.At(1).Gauge().DataPoints().At(1).DoubleValue(), float64(3))
	require.EqualValues(t, all.At(1).Gauge().DataPoints().At(1).Attributes().AsRaw(), map[string]any{
		"w": "n",
	})
}

func TestDatadogLogsServer(t *testing.T) {
	server := DatadogLogServerMock()
	values := JSONLogs{
		{
			"company":   "datadog",
			"component": "logs",
		},
	}
	jsonBytes, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
		return
	}
	var buf = bytes.NewBuffer([]byte{})
	w := gzip.NewWriter(buf)
	_, _ = w.Write(jsonBytes)
	_ = w.Close()
	resp, err := http.Post(server.URL, "application/json", buf)
	defer func() {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
	}()
	if err != nil {
		t.Fatal(err)
		return
	}
	assert.Equal(t, 202, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
		return
	}
	assert.Equal(t, []byte(`{"status":"ok"}`), body)
	assert.Equal(t, values, server.LogsData)

}
