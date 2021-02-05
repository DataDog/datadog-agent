// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package status

import (
	"reflect"
	"testing"
)

func Test_convertExpvarRunnerStats(t *testing.T) {
	tests := []struct {
		name      string
		inputJSON []byte
		want      CLCChecks
		wantErr   bool
	}{
		{
			name:      "no error present",
			inputJSON: []byte(`{"Checks": {"foo": {"id1": {"AverageExecutionTime": 42, "MetricSamples": 100, "LastError": ""}}}}`),
			want: CLCChecks{
				Checks: map[string]map[string]CLCStats{
					"foo": {
						"id1": {
							AverageExecutionTime: 42,
							MetricSamples:        100,
							LastExecFailed:       false,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name:      "error present",
			inputJSON: []byte(`{"Checks": {"foo": {"id1": {"AverageExecutionTime": 42, "MetricSamples": 100, "LastError": "this is an error"}}}}`),
			want: CLCChecks{
				Checks: map[string]map[string]CLCStats{
					"foo": {
						"id1": {
							AverageExecutionTime: 42,
							MetricSamples:        100,
							LastExecFailed:       true,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name:      "bad json",
			inputJSON: []byte(`{"Checks": bad-json{}}`),
			want:      CLCChecks{},
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := convertExpvarRunnerStats(tt.inputJSON)
			if (err != nil) != tt.wantErr {
				t.Errorf("convertExpvarRunnerStats() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("convertExpvarRunnerStats() = %v, want %v", got, tt.want)
			}
		})
	}
}
