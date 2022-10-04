// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package system

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	utilsystem "github.com/DataDog/datadog-agent/pkg/util/system"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

func TestBuildContainerMetrics(t *testing.T) {
	tests := []struct {
		name string
		cg   cgroups.Cgroup
		want *provider.ContainerStats
	}{
		{
			name: "everything empty",
			cg:   &cgroups.MockCgroup{},
			want: &provider.ContainerStats{
				PID: &provider.ContainerPIDStats{},
			},
		},
		{
			name: "structs with all stats",
			cg: &cgroups.MockCgroup{
				CPU: &cgroups.CPUStats{
					Total:            pointer.UInt64Ptr(100),
					System:           pointer.UInt64Ptr(200),
					User:             pointer.UInt64Ptr(300),
					Shares:           pointer.UInt64Ptr(400),
					ElapsedPeriods:   pointer.UInt64Ptr(500),
					ThrottledPeriods: pointer.UInt64Ptr(0),
					ThrottledTime:    pointer.UInt64Ptr(100),
					CPUCount:         pointer.UInt64Ptr(10),
					SchedulerPeriod:  pointer.UInt64Ptr(100),
					SchedulerQuota:   pointer.UInt64Ptr(50),
				},
				Memory: &cgroups.MemoryStats{
					UsageTotal:   pointer.UInt64Ptr(100),
					KernelMemory: pointer.UInt64Ptr(40),
					Limit:        pointer.UInt64Ptr(42000),
					LowThreshold: pointer.UInt64Ptr(40000),
					RSS:          pointer.UInt64Ptr(300),
					Cache:        pointer.UInt64Ptr(200),
					Swap:         pointer.UInt64Ptr(0),
					SwapLimit:    pointer.UInt64Ptr(500),
					OOMEvents:    pointer.UInt64Ptr(10),
				},
				IOStats: &cgroups.IOStats{
					ReadBytes:       pointer.UInt64Ptr(100),
					WriteBytes:      pointer.UInt64Ptr(200),
					ReadOperations:  pointer.UInt64Ptr(10),
					WriteOperations: pointer.UInt64Ptr(20),
					// Device will be ignored as no matching device name
					Devices: map[string]cgroups.DeviceIOStats{
						"foo": {
							ReadBytes:       pointer.UInt64Ptr(100),
							WriteBytes:      pointer.UInt64Ptr(200),
							ReadOperations:  pointer.UInt64Ptr(10),
							WriteOperations: pointer.UInt64Ptr(20),
						},
					},
				},
				PIDStats: &cgroups.PIDStats{
					HierarchicalThreadCount: pointer.UInt64Ptr(10),
					HierarchicalThreadLimit: pointer.UInt64Ptr(20),
				},
				PIDs: []int{4, 2},
			},
			want: &provider.ContainerStats{
				CPU: &provider.ContainerCPUStats{
					Total:            pointer.Float64Ptr(100),
					System:           pointer.Float64Ptr(200),
					User:             pointer.Float64Ptr(300),
					Shares:           pointer.Float64Ptr(400),
					Limit:            pointer.Float64Ptr(50),
					ElapsedPeriods:   pointer.Float64Ptr(500),
					ThrottledPeriods: pointer.Float64Ptr(0),
					ThrottledTime:    pointer.Float64Ptr(100),
				},
				Memory: &provider.ContainerMemStats{
					UsageTotal:   pointer.Float64Ptr(100),
					KernelMemory: pointer.Float64Ptr(40),
					Limit:        pointer.Float64Ptr(42000),
					Softlimit:    pointer.Float64Ptr(40000),
					RSS:          pointer.Float64Ptr(300),
					Cache:        pointer.Float64Ptr(200),
					Swap:         pointer.Float64Ptr(0),
					SwapLimit:    pointer.Float64Ptr(500),
					OOMEvents:    pointer.Float64Ptr(10),
				},
				IO: &provider.ContainerIOStats{
					ReadBytes:       pointer.Float64Ptr(100),
					WriteBytes:      pointer.Float64Ptr(200),
					ReadOperations:  pointer.Float64Ptr(10),
					WriteOperations: pointer.Float64Ptr(20),
				},
				PID: &provider.ContainerPIDStats{
					PIDs:        []int{4, 2},
					ThreadCount: pointer.Float64Ptr(10),
					ThreadLimit: pointer.Float64Ptr(20),
				},
			},
		},
		{
			name: "limit cpu count no quota",
			cg: &cgroups.MockCgroup{
				CPU: &cgroups.CPUStats{
					CPUCount: pointer.UInt64Ptr(10),
				},
			},
			want: &provider.ContainerStats{
				CPU: &provider.ContainerCPUStats{
					Limit: pointer.Float64Ptr(1000),
				},
				PID: &provider.ContainerPIDStats{},
			},
		},
		{
			name: "limit no cpu count, no quota",
			cg: &cgroups.MockCgroup{
				CPU: &cgroups.CPUStats{},
			},
			want: &provider.ContainerStats{
				CPU: &provider.ContainerCPUStats{
					Limit: pointer.Float64Ptr(float64(utilsystem.HostCPUCount()) * 100),
				},
				PID: &provider.ContainerPIDStats{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &systemCollector{}
			got, err := c.buildContainerMetrics(tt.cg, 0)
			assert.NoError(t, err)
			tt.want.Timestamp = got.Timestamp
			assert.Empty(t, cmp.Diff(tt.want, got))
		})
	}
}
