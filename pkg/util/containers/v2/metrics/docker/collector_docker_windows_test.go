// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker && windows
// +build docker,windows

package docker

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics"
	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/assert"
)

func Test_convertCPUStats(t *testing.T) {
	tests := []struct {
		name           string
		input          types.CPUStats
		expectedOutput metrics.ContainerCPUStats
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
			expectedOutput: metrics.ContainerCPUStats{
				Total:  util.Float64Ptr(4200),
				System: util.Float64Ptr(4300),
				User:   util.Float64Ptr(4400),
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
		expectedOutput metrics.ContainerMemStats
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
			expectedOutput: metrics.ContainerMemStats{
				UsageTotal:        util.Float64Ptr(42),
				Limit:             util.Float64Ptr(43),
				PrivateWorkingSet: util.Float64Ptr(46),
				CommitBytes:       util.Float64Ptr(44),
				CommitPeakBytes:   util.Float64Ptr(45),
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
		expectedOutput metrics.ContainerIOStats
	}{
		{
			name: "basic",
			input: types.StorageStats{
				ReadCountNormalized:  42,
				ReadSizeBytes:        43,
				WriteCountNormalized: 44,
				WriteSizeBytes:       45,
			},
			expectedOutput: metrics.ContainerIOStats{
				ReadBytes:       util.Float64Ptr(43),
				WriteBytes:      util.Float64Ptr(45),
				ReadOperations:  util.Float64Ptr(42),
				WriteOperations: util.Float64Ptr(44),
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
		expectedOutput metrics.ContainerPIDStats
	}{
		{
			name:  "basic",
			input: 42,
			expectedOutput: metrics.ContainerPIDStats{
				ThreadCount: util.Float64Ptr(42),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, &test.expectedOutput, convertPIDStats(test.input))
		})
	}
}
