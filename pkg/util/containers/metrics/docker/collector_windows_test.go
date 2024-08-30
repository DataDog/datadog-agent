// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker && windows

package docker

import (
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/pkg/util/system"
)

func Test_convertCPUStats(t *testing.T) {
	tests := []struct {
		name           string
		input          types.CPUStats
		expectedOutput provider.ContainerCPUStats
	}{
		{
			name: "basic",
			input: types.CPUStats{
				CPUUsage: types.CPUUsage{
					TotalUsage:        42,
					UsageInKernelmode: 43,
					UsageInUsermode:   44,
				},
			},
			expectedOutput: provider.ContainerCPUStats{
				Total:  pointer.Ptr(4200.0),
				System: pointer.Ptr(4300.0),
				User:   pointer.Ptr(4400.0),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, &test.expectedOutput, convertCPUStats(&test.input))
		})
	}
}

func Test_convertMemoryStats(t *testing.T) {
	tests := []struct {
		name           string
		input          types.MemoryStats
		expectedOutput provider.ContainerMemStats
	}{
		{
			name: "basic",
			input: types.MemoryStats{
				Commit:            44,
				CommitPeak:        45,
				PrivateWorkingSet: 46,
			},
			expectedOutput: provider.ContainerMemStats{
				UsageTotal:        pointer.Ptr(44.0),
				PrivateWorkingSet: pointer.Ptr(46.0),
				CommitBytes:       pointer.Ptr(44.0),
				CommitPeakBytes:   pointer.Ptr(45.0),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, &test.expectedOutput, convertMemoryStats(&test.input))
		})
	}
}

func Test_convertIOStats(t *testing.T) {
	tests := []struct {
		name           string
		input          types.StorageStats
		expectedOutput provider.ContainerIOStats
	}{
		{
			name: "basic",
			input: types.StorageStats{
				ReadCountNormalized:  42,
				ReadSizeBytes:        43,
				WriteCountNormalized: 44,
				WriteSizeBytes:       45,
			},
			expectedOutput: provider.ContainerIOStats{
				ReadBytes:       pointer.Ptr(43.0),
				WriteBytes:      pointer.Ptr(45.0),
				ReadOperations:  pointer.Ptr(42.0),
				WriteOperations: pointer.Ptr(44.0),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, &test.expectedOutput, convertIOStats(&test.input))
		})
	}
}

func Test_convetrPIDStats(t *testing.T) {
	tests := []struct {
		name           string
		input          uint32
		expectedOutput provider.ContainerPIDStats
	}{
		{
			name:  "basic",
			input: 42,
			expectedOutput: provider.ContainerPIDStats{
				ThreadCount: pointer.Ptr(42.0),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, &test.expectedOutput, convertPIDStats(test.input))
		})
	}
}

func Test_computeCPULimit(t *testing.T) {
	tests := []struct {
		name          string
		spec          *types.ContainerJSON
		expectedLimit float64
	}{
		{
			name: "No CPU Limit",
			spec: &types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					HostConfig: &container.HostConfig{},
				},
			},
			expectedLimit: 100 * float64(system.HostCPUCount()),
		},
		{
			name: "Nano CPUs",
			spec: &types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					HostConfig: &container.HostConfig{
						Resources: container.Resources{
							NanoCPUs: 5000000000,
						},
					},
				},
			},
			expectedLimit: 500,
		},
		{
			name: "CPU Percent",
			spec: &types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					HostConfig: &container.HostConfig{
						Resources: container.Resources{
							CPUPercent: 50,
						},
					},
				},
			},
			expectedLimit: 50 * float64(system.HostCPUCount()),
		},
		{
			name: "CPU Count",
			spec: &types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					HostConfig: &container.HostConfig{
						Resources: container.Resources{
							CPUCount: 2,
						},
					},
				},
			},
			expectedLimit: 200,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			containerStats := &provider.ContainerStats{CPU: &provider.ContainerCPUStats{}}
			computeCPULimit(containerStats, tt.spec)
			assert.Equal(t, tt.expectedLimit, *containerStats.CPU.Limit)
		})
	}
}
