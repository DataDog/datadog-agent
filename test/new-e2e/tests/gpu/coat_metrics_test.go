// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package gpu

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCOATMetrics(t *testing.T) {
	const payload = `{"request_type":"agent-metrics","payload":{"metrics":{"agent_metadata":{"value":1},"gpu.foo":{"value":2,"tags":{"a":"b"}}}}}`
	const batch = `{"request_type":"message-batch","payload":[` +
		`{"request_type":"agent-metrics","payload":{"metrics":{"gpu.foo":{"value":1,"tags":{"gpu":"0"}}}}},` +
		`{"request_type":"agent-metrics","payload":{"metrics":{"gpu.foo":{"value":3,"tags":{"gpu":"1"}}}}},` +
		`{"request_type":"other","payload":{}}]}`

	tests := []struct {
		name    string
		output  string
		want    map[string][]float64
		wantErr string
	}{
		{name: "single payload", output: payload, want: map[string][]float64{"gpu.foo": {2}}},
		{
			name:   "log lines with braces around the payload",
			output: "debug: converting value from []interface {} to []string\n" + payload + "\ntrailing {text}\n",
			want:   map[string][]float64{"gpu.foo": {2}},
		},
		{name: "batch spreads a metric across messages", output: batch, want: map[string][]float64{"gpu.foo": {1, 3}}},
		{name: "no payload", output: "nothing to see", wantErr: "no JSON payload"},
		{name: "unexpected request type", output: `{"request_type":"foo","payload":{}}`, wantErr: "unexpected agent telemetry request type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics, err := parseCOATMetrics(tt.output)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			got := make(map[string][]float64)
			for name, ms := range metrics {
				for _, m := range ms {
					got[name] = append(got[name], m.Value)
				}
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
