// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker && linux
// +build docker,linux

package docker

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/pkg/util/system"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
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
				ThrottlingData: types.ThrottlingData{
					ThrottledPeriods: 45,
					ThrottledTime:    46,
				},
			},
			expectedOutput: provider.ContainerCPUStats{
				Total:            pointer.Float64Ptr(42),
				System:           pointer.Float64Ptr(43),
				User:             pointer.Float64Ptr(44),
				ThrottledPeriods: pointer.Float64Ptr(45),
				ThrottledTime:    pointer.Float64Ptr(46),
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
				Usage:   42,
				Limit:   43,
				Failcnt: 44,
				Stats: map[string]uint64{
					"rss":          45,
					"cache":        46,
					"kernel_stack": 47,
					"slab":         48,
				},
			},
			expectedOutput: provider.ContainerMemStats{
				UsageTotal:   pointer.Float64Ptr(42),
				KernelMemory: pointer.Float64Ptr(95),
				Limit:        pointer.Float64Ptr(43),
				OOMEvents:    pointer.Float64Ptr(44),
				RSS:          pointer.Float64Ptr(45),
				Cache:        pointer.Float64Ptr(46),
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
		input          types.BlkioStats
		expectedOutput provider.ContainerIOStats
	}{
		{
			name: "basic",
			input: types.BlkioStats{
				IoServiceBytesRecursive: []types.BlkioStatEntry{
					{
						Major: 1,
						Minor: 2,
						Op:    "Read",
						Value: 42,
					},
					{
						Major: 1,
						Minor: 2,
						Op:    "Write",
						Value: 43,
					},
					{
						Major: 1,
						Minor: 3,
						Op:    "Read",
						Value: 44,
					},
					{
						Major: 1,
						Minor: 3,
						Op:    "Write",
						Value: 45,
					},
				},
				IoServicedRecursive: []types.BlkioStatEntry{
					{
						Major: 1,
						Minor: 2,
						Op:    "Read",
						Value: 46,
					},
					{
						Major: 1,
						Minor: 2,
						Op:    "Write",
						Value: 47,
					},
					{
						Major: 1,
						Minor: 3,
						Op:    "Read",
						Value: 48,
					},
					{
						Major: 1,
						Minor: 3,
						Op:    "Write",
						Value: 49,
					},
				},
			},
			expectedOutput: provider.ContainerIOStats{
				ReadBytes:       pointer.Float64Ptr(86),
				WriteBytes:      pointer.Float64Ptr(88),
				ReadOperations:  pointer.Float64Ptr(94),
				WriteOperations: pointer.Float64Ptr(96),
				Devices: map[string]provider.DeviceIOStats{
					"foo1": {
						ReadBytes:       pointer.Float64Ptr(42),
						WriteBytes:      pointer.Float64Ptr(43),
						ReadOperations:  pointer.Float64Ptr(46),
						WriteOperations: pointer.Float64Ptr(47),
					},
					"bar2": {
						ReadBytes:       pointer.Float64Ptr(44),
						WriteBytes:      pointer.Float64Ptr(45),
						ReadOperations:  pointer.Float64Ptr(48),
						WriteOperations: pointer.Float64Ptr(49),
					},
				},
			},
		},
	}

	dir := t.TempDir()

	diskstats := []byte(
		"   1       2 foo1 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0\n" +
			"   1       3 bar2 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0",
	)

	err := ioutil.WriteFile(dir+"/diskstats", diskstats, 0o644)
	assert.Nil(t, err)
	defer os.Remove(dir + "/diskstats")

	config.Datadog.Set("container_proc_root", dir)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, &test.expectedOutput, convertIOStats(&test.input))
		})
	}
}

func Test_convetrPIDStats(t *testing.T) {
	tests := []struct {
		name           string
		input          types.PidsStats
		expectedOutput provider.ContainerPIDStats
	}{
		{
			name: "basic",
			input: types.PidsStats{
				Current: 42,
				Limit:   43,
			},
			expectedOutput: provider.ContainerPIDStats{
				ThreadCount: pointer.Float64Ptr(42),
				ThreadLimit: pointer.Float64Ptr(43),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, &test.expectedOutput, convertPIDStats(&test.input))
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
			name: "CFS Quotas with period",
			spec: &types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					HostConfig: &container.HostConfig{
						Resources: container.Resources{
							CPUPeriod: 10000,
							CPUQuota:  5000,
						},
					},
				},
			},
			expectedLimit: 50,
		},
		{
			name: "CFS Quotas without period",
			spec: &types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					HostConfig: &container.HostConfig{
						Resources: container.Resources{
							CPUQuota: 5000,
						},
					},
				},
			},
			expectedLimit: 5,
		},
		{
			name: "CPU Set",
			spec: &types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					HostConfig: &container.HostConfig{
						Resources: container.Resources{
							CpusetCpus: "0-2",
						},
					},
				},
			},
			expectedLimit: 300,
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
