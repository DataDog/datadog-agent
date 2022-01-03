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

	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics/provider"
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
				Total:            util.Float64Ptr(42),
				System:           util.Float64Ptr(43),
				User:             util.Float64Ptr(44),
				ThrottledPeriods: util.Float64Ptr(45),
				ThrottledTime:    util.Float64Ptr(46),
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
				UsageTotal:   util.Float64Ptr(42),
				KernelMemory: util.Float64Ptr(95),
				Limit:        util.Float64Ptr(43),
				OOMEvents:    util.Float64Ptr(44),
				RSS:          util.Float64Ptr(45),
				Cache:        util.Float64Ptr(46),
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
				ReadBytes:       util.Float64Ptr(86),
				WriteBytes:      util.Float64Ptr(88),
				ReadOperations:  util.Float64Ptr(94),
				WriteOperations: util.Float64Ptr(96),
				Devices: map[string]provider.DeviceIOStats{
					"foo1": {
						ReadBytes:       util.Float64Ptr(42),
						WriteBytes:      util.Float64Ptr(43),
						ReadOperations:  util.Float64Ptr(46),
						WriteOperations: util.Float64Ptr(47),
					},
					"bar2": {
						ReadBytes:       util.Float64Ptr(44),
						WriteBytes:      util.Float64Ptr(45),
						ReadOperations:  util.Float64Ptr(48),
						WriteOperations: util.Float64Ptr(49),
					},
				},
			},
		},
	}

	dir, err := os.MkdirTemp("", "proc")
	assert.Nil(t, err)
	defer os.Remove(dir)

	diskstats := []byte(
		"   1       2 foo1 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0\n" +
			"   1       3 bar2 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0",
	)

	err = ioutil.WriteFile(dir+"/diskstats", diskstats, 0644)
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
				ThreadCount: util.Float64Ptr(42),
				ThreadLimit: util.Float64Ptr(43),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, &test.expectedOutput, convertPIDStats(&test.input))
		})
	}
}
