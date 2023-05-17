// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
)

func Test_busynessFunc(t *testing.T) {
	tests := []struct {
		name  string
		stats types.CLCRunnerStats
		want  int
	}{
		{
			name: "nominal case",
			stats: types.CLCRunnerStats{
				AverageExecutionTime: 100,
				MetricSamples:        100,
			},
			want: 100,
		},
		{
			name: "cluster check",
			stats: types.CLCRunnerStats{
				AverageExecutionTime: 100,
				MetricSamples:        100,
				IsClusterCheck:       true,
			},
			want: 100,
		},
		{
			name: "node based check",
			stats: types.CLCRunnerStats{
				AverageExecutionTime: 100,
				MetricSamples:        100,
				IsClusterCheck:       false,
			},
			want: 100,
		},
		{
			name: "failed check",
			stats: types.CLCRunnerStats{
				AverageExecutionTime: 100,
				MetricSamples:        100,
				LastExecFailed:       true,
			},
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := busynessFunc(tt.stats); got != tt.want {
				t.Errorf("busynessFunc() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_calculateBusyness(t *testing.T) {
	tests := []struct {
		name  string
		stats types.CLCRunnersStats
		want  int
	}{
		{
			name: "nominal case",
			stats: types.CLCRunnersStats{
				"cluster check": types.CLCRunnerStats{
					AverageExecutionTime: 100,
					MetricSamples:        100,
					IsClusterCheck:       true,
				},
				"node check": types.CLCRunnerStats{
					AverageExecutionTime: 100,
					MetricSamples:        100,
					IsClusterCheck:       false,
				},
				"failed check": types.CLCRunnerStats{
					AverageExecutionTime: 100,
					MetricSamples:        100,
					LastExecFailed:       true,
				},
			},
			want: 200,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := calculateBusyness(tt.stats); got != tt.want {
				t.Errorf("calculateBusyness() = %v, want %v", got, tt.want)
			}
		})
	}
}
