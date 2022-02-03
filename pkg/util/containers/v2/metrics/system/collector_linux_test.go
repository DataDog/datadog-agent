// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package system

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics/provider"
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
					Total:            util.UInt64Ptr(100),
					System:           util.UInt64Ptr(200),
					User:             util.UInt64Ptr(300),
					Shares:           util.UInt64Ptr(400),
					ElapsedPeriods:   util.UInt64Ptr(500),
					ThrottledPeriods: util.UInt64Ptr(0),
					ThrottledTime:    util.UInt64Ptr(100),
					CPUCount:         util.UInt64Ptr(10),
					SchedulerPeriod:  util.UInt64Ptr(100),
					SchedulerQuota:   util.UInt64Ptr(50),
				},
				Memory: &cgroups.MemoryStats{
					UsageTotal:   util.UInt64Ptr(100),
					KernelMemory: util.UInt64Ptr(40),
					Limit:        util.UInt64Ptr(42000),
					LowThreshold: util.UInt64Ptr(40000),
					RSS:          util.UInt64Ptr(300),
					Cache:        util.UInt64Ptr(200),
					Swap:         util.UInt64Ptr(0),
					OOMEvents:    util.UInt64Ptr(10),
				},
				IOStats: &cgroups.IOStats{
					ReadBytes:       util.UInt64Ptr(100),
					WriteBytes:      util.UInt64Ptr(200),
					ReadOperations:  util.UInt64Ptr(10),
					WriteOperations: util.UInt64Ptr(20),
					// Device will be ignored as no matching device name
					Devices: map[string]cgroups.DeviceIOStats{
						"foo": {
							ReadBytes:       util.UInt64Ptr(100),
							WriteBytes:      util.UInt64Ptr(200),
							ReadOperations:  util.UInt64Ptr(10),
							WriteOperations: util.UInt64Ptr(20),
						},
					},
				},
				PIDStats: &cgroups.PIDStats{
					HierarchicalThreadCount: util.UInt64Ptr(10),
					HierarchicalThreadLimit: util.UInt64Ptr(20),
				},
				PIDs: []int{4, 2},
			},
			want: &provider.ContainerStats{
				CPU: &provider.ContainerCPUStats{
					Total:            util.Float64Ptr(100),
					System:           util.Float64Ptr(200),
					User:             util.Float64Ptr(300),
					Shares:           util.Float64Ptr(400),
					Limit:            util.Float64Ptr(50),
					ElapsedPeriods:   util.Float64Ptr(500),
					ThrottledPeriods: util.Float64Ptr(0),
					ThrottledTime:    util.Float64Ptr(100),
				},
				Memory: &provider.ContainerMemStats{
					UsageTotal:   util.Float64Ptr(100),
					KernelMemory: util.Float64Ptr(40),
					Limit:        util.Float64Ptr(42000),
					Softlimit:    util.Float64Ptr(40000),
					RSS:          util.Float64Ptr(300),
					Cache:        util.Float64Ptr(200),
					Swap:         util.Float64Ptr(0),
					OOMEvents:    util.Float64Ptr(10),
				},
				IO: &provider.ContainerIOStats{
					ReadBytes:       util.Float64Ptr(100),
					WriteBytes:      util.Float64Ptr(200),
					ReadOperations:  util.Float64Ptr(10),
					WriteOperations: util.Float64Ptr(20),
				},
				PID: &provider.ContainerPIDStats{
					PIDs:        []int{4, 2},
					ThreadCount: util.Float64Ptr(10),
					ThreadLimit: util.Float64Ptr(20),
				},
			},
		},
		{
			name: "limit cpu count no quota",
			cg: &cgroups.MockCgroup{
				CPU: &cgroups.CPUStats{
					CPUCount: util.UInt64Ptr(10),
				},
			},
			want: &provider.ContainerStats{
				CPU: &provider.ContainerCPUStats{
					Limit: util.Float64Ptr(1000),
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
					Limit: util.Float64Ptr(float64(utilsystem.HostCPUCount()) * 100),
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
