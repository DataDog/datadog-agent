// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clustercheckimpl

import (
	"testing"

	clusterchecks "github.com/DataDog/datadog-agent/comp/core/clusterchecks/def"
)

func Test_busynessFunc(t *testing.T) {
	tests := []struct {
		name  string
		stats clusterchecks.CLCRunnerStats
		want  int
	}{
		{
			name: "nominal case",
			stats: clusterchecks.CLCRunnerStats{
				AverageExecutionTime: 100,
				MetricSamples:        100,
			},
			want: 100,
		},
		{
			name: "cluster check",
			stats: clusterchecks.CLCRunnerStats{
				AverageExecutionTime: 100,
				MetricSamples:        100,
				IsClusterCheck:       true,
			},
			want: 100,
		},
		{
			name: "node based check",
			stats: clusterchecks.CLCRunnerStats{
				AverageExecutionTime: 100,
				MetricSamples:        100,
				IsClusterCheck:       false,
			},
			want: 100,
		},
		{
			name: "failed check",
			stats: clusterchecks.CLCRunnerStats{
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
		stats clusterchecks.CLCRunnersStats
		want  int
	}{
		{
			name: "nominal case",
			stats: clusterchecks.CLCRunnersStats{
				"cluster check": clusterchecks.CLCRunnerStats{
					AverageExecutionTime: 100,
					MetricSamples:        100,
					IsClusterCheck:       true,
				},
				"node check": clusterchecks.CLCRunnerStats{
					AverageExecutionTime: 100,
					MetricSamples:        100,
					IsClusterCheck:       false,
				},
				"failed check": clusterchecks.CLCRunnerStats{
					AverageExecutionTime: 100,
					MetricSamples:        100,
					LastExecFailed:       true,
				},
			},
			want: 200,
		},
		{
			name: "with histogrambuckets case",
			stats: clusterchecks.CLCRunnersStats{
				"cluster check": clusterchecks.CLCRunnerStats{
					AverageExecutionTime: 100,
					MetricSamples:        100,
					HistogramBuckets:     100,
					IsClusterCheck:       true,
				},
				"node check": clusterchecks.CLCRunnerStats{
					AverageExecutionTime: 100,
					MetricSamples:        100,
					HistogramBuckets:     100,
					IsClusterCheck:       false,
				},
				"failed check": clusterchecks.CLCRunnerStats{
					AverageExecutionTime: 100,
					MetricSamples:        100,
					HistogramBuckets:     100,
					LastExecFailed:       true,
				},
			},
			want: 400,
		},
		{
			name: "with events case",
			stats: clusterchecks.CLCRunnersStats{
				"cluster check": clusterchecks.CLCRunnerStats{
					AverageExecutionTime: 100,
					MetricSamples:        100,
					HistogramBuckets:     100,
					Events:               100,
					IsClusterCheck:       true,
				},
				"node check": clusterchecks.CLCRunnerStats{
					AverageExecutionTime: 100,
					MetricSamples:        100,
					HistogramBuckets:     100,
					Events:               100,
					IsClusterCheck:       false,
				},
				"failed check": clusterchecks.CLCRunnerStats{
					AverageExecutionTime: 100,
					MetricSamples:        100,
					HistogramBuckets:     100,
					Events:               100,
					LastExecFailed:       true,
				},
			},
			want: 440,
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
