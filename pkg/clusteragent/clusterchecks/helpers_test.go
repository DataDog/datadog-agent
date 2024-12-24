// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
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
		{
			name: "with histogrambuckets case",
			stats: types.CLCRunnersStats{
				"cluster check": types.CLCRunnerStats{
					AverageExecutionTime: 100,
					MetricSamples:        100,
					HistogramBuckets:     100,
					IsClusterCheck:       true,
				},
				"node check": types.CLCRunnerStats{
					AverageExecutionTime: 100,
					MetricSamples:        100,
					HistogramBuckets:     100,
					IsClusterCheck:       false,
				},
				"failed check": types.CLCRunnerStats{
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
			stats: types.CLCRunnersStats{
				"cluster check": types.CLCRunnerStats{
					AverageExecutionTime: 100,
					MetricSamples:        100,
					HistogramBuckets:     100,
					Events:               100,
					IsClusterCheck:       true,
				},
				"node check": types.CLCRunnerStats{
					AverageExecutionTime: 100,
					MetricSamples:        100,
					HistogramBuckets:     100,
					Events:               100,
					IsClusterCheck:       false,
				},
				"failed check": types.CLCRunnerStats{
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

func Test_scanExtendedDanglingConfigs(t *testing.T) {
	attemptLimit := 3
	store := newClusterStore()
	c1 := createDanglingConfig(integration.Config{
		Name:   "config1",
		Source: "source1",
	})
	store.danglingConfigs[c1.config.Digest()] = c1

	for i := 0; i < attemptLimit; i++ {
		scanExtendedDanglingConfigs(store, attemptLimit)
	}

	assert.Equal(t, attemptLimit, c1.rescheduleAttempts)
	assert.False(t, c1.detectedExtendedDangling)
	assert.False(t, c1.isStuckScheduling(attemptLimit))

	scanExtendedDanglingConfigs(store, attemptLimit)

	assert.Equal(t, attemptLimit+1, c1.rescheduleAttempts)
	assert.True(t, c1.detectedExtendedDangling)
	assert.True(t, c1.isStuckScheduling(attemptLimit))
}
