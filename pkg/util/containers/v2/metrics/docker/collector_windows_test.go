// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker && windows
// +build docker,windows

package docker

import (
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
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
				Total:  pointer.Float64Ptr(4200),
				System: pointer.Float64Ptr(4300),
				User:   pointer.Float64Ptr(4400),
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
				Usage:             42,
				Limit:             43,
				Commit:            44,
				CommitPeak:        45,
				PrivateWorkingSet: 46,
			},
			expectedOutput: provider.ContainerMemStats{
				UsageTotal:        pointer.Float64Ptr(42),
				Limit:             pointer.Float64Ptr(43),
				PrivateWorkingSet: pointer.Float64Ptr(46),
				CommitBytes:       pointer.Float64Ptr(44),
				CommitPeakBytes:   pointer.Float64Ptr(45),
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
				ReadBytes:       pointer.Float64Ptr(43),
				WriteBytes:      pointer.Float64Ptr(45),
				ReadOperations:  pointer.Float64Ptr(42),
				WriteOperations: pointer.Float64Ptr(44),
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
				ThreadCount: pointer.Float64Ptr(42),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, &test.expectedOutput, convertPIDStats(test.input))
		})
	}
}
