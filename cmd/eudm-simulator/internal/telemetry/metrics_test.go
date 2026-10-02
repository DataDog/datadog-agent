// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package telemetry

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

func TestMetricRoundTripRetainsAgentOriginAndRelativeTime(t *testing.T) {
	input := []*metrics.Serie{{Name: "system.wlan.rssi", Host: "capture-host", Device: "device-1", Source: metrics.MetricSourceWlan, MType: metrics.APIGaugeType, Interval: 15, Unit: "dBm", SourceTypeName: "System", NoIndex: true, Resources: []metrics.Resource{{Type: "host", Name: "native-host"}}, Tags: tagset.CompositeTagsFromSlice([]string{"bssid:02:00:00:00:00:01"}), Points: []metrics.Point{{Ts: -0.125, Value: -54.5}, {Ts: 15.3125, Value: -53}}}}
	envelope, err := NewMetricSample(input)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(schema.Metrics, data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(input, decoded.Metrics) {
		t.Fatal("Agent origin, fractional points, or series identity changed")
	}
	// The envelope and reconstructed Agent types own their data independently.
	input[0].Points[0].Value = 99
	input[0].Source = metrics.MetricSourceCPU
	input[0].Resources[0].Name = "mutated"
	decoded.Metrics[0].Resources[0].Type = "mutated"
	if envelope.Series[0].Resources[0].Name != "native-host" || envelope.Series[0].Resources[0].Type != "host" {
		t.Fatal("metric resources alias mutable source or replay data")
	}
	decoded.Metrics[0].Points[1].Value = 100
	if envelope.Series[0].Points[0].Value != -54.5 || envelope.Series[0].Points[1].Value != -53 || *envelope.Series[0].Source != metrics.MetricSourceWlan {
		t.Fatal("metric envelope aliases mutable source or replay data")
	}
}

func TestRejectIncompleteOrInvalidMetricEnvelope(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"missing source", func(v map[string]any) { delete(v, "source") }},
		{"null source", func(v map[string]any) { v["source"] = nil }},
		{"unknown source", func(v map[string]any) { v["source"] = 65535 }},
		{"unknown type", func(v map[string]any) { v["type"] = "not-a-metric-type" }},
		{"missing type", func(v map[string]any) { delete(v, "type") }},
		{"missing points", func(v map[string]any) { delete(v, "points") }},
		{"null points", func(v map[string]any) { v["points"] = nil }},
		{"missing host", func(v map[string]any) { delete(v, "host") }},
		{"unknown field", func(v map[string]any) { v["unexpected"] = "value" }},
		{"unknown point field", func(v map[string]any) {
			v["points"] = []any{map[string]any{"timestamp": 0, "value": 3, "unexpected": "value"}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			series := map[string]any{"metric": "system.cpu.user", "host": "capture-host", "type": "gauge", "source": metrics.MetricSourceCPU, "points": []any{map[string]any{"timestamp": 0, "value": 3}}}
			tc.mutate(series)
			data, err := json.Marshal(map[string]any{"series": []any{series}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Decode(schema.Metrics, data); err == nil {
				t.Fatal("accepted invalid metric envelope")
			}
		})
	}
}
