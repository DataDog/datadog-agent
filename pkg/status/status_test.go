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
			inputJSON: []byte(`{"Checks": {"foo": {"id1": {"AverageExecutionTime": 42, "MetricSamples": 100, "HistogramBuckets": 50, "Events": 200, "LastError": "", "UpdateTimestamp": "2026-04-07T12:00:00Z"}}}}`),
			want: CLCChecks{
				Checks: map[string]map[string]CLCStats{
					"foo": {
						"id1": {
							AverageExecutionTime: 42,
							MetricSamples:        100,
							HistogramBuckets:     50,
							Events:               200,
							LastExecFailed:       false,
							LastExecutionDate:    1775563200000,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name:      "error present",
			inputJSON: []byte(`{"Checks": {"foo": {"id1": {"AverageExecutionTime": 42, "MetricSamples": 100, "HistogramBuckets": 50, "Events": 200, "LastError": "this is an error", "UpdateTimestamp": "2026-04-07T12:00:00Z"}}}}`),
			want: CLCChecks{
				Checks: map[string]map[string]CLCStats{
					"foo": {
						"id1": {
							AverageExecutionTime: 42,
							MetricSamples:        100,
							HistogramBuckets:     50,
							Events:               200,
							LastExecFailed:       true,
							LastError:            "this is an error",
							LastExecutionDate:    1775563200000,
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
		{
			name:      "all execution status fields",
			inputJSON: []byte(`{"Checks": {"bar": {"id2": {"AverageExecutionTime": 500, "MetricSamples": 10, "HistogramBuckets": 5, "Events": 3, "ServiceChecks": 7, "TotalRuns": 100, "TotalErrors": 2, "TotalMetricSamples": 1000, "TotalEvents": 300, "TotalServiceChecks": 700, "LastSuccessDate": 1775563775, "UpdateTimestamp": "2026-04-07T12:09:35.974Z", "LastError": ""}}}}`),
			want: CLCChecks{
				Checks: map[string]map[string]CLCStats{
					"bar": {
						"id2": {
							AverageExecutionTime: 500,
							MetricSamples:        10,
							HistogramBuckets:     5,
							Events:               3,
							ServiceChecks:        7,
							LastExecFailed:       false,
							TotalRuns:            100,
							TotalErrors:          2,
							TotalMetricSamples:   1000,
							TotalEvents:          300,
							TotalServiceChecks:   700,
							LastSuccessDate:      1775563775,
							LastExecutionDate:    1775563775974,
						},
					},
				},
			},
			wantErr: false,
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
