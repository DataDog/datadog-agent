// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package system

import (
	"errors"
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
		name      string
		cg        cgroups.Cgroup
		wantStats *provider.ContainerStats
		wantErr   bool
	}{
		{
			name: "everything empty",
			cg: &cgroups.MockCgroup{
				CPUError:    errors.New("not found"),
				MemoryError: errors.New("not found"),
				IOError:     errors.New("not found"),
				PIDError:    errors.New("not found"),
			},
			wantStats: nil,
			wantErr:   true,
		},
		{
			name: "structs with all stats",
			cg: &cgroups.MockCgroup{
				CPU: &cgroups.CPUStats{
					Total:            pointer.Ptr(uint64(100)),
					System:           pointer.Ptr(uint64(200)),
					User:             pointer.Ptr(uint64(300)),
					Shares:           pointer.Ptr(uint64(400)),
					ElapsedPeriods:   pointer.Ptr(uint64(500)),
					ThrottledPeriods: pointer.Ptr(uint64(0)),
					ThrottledTime:    pointer.Ptr(uint64(100)),
					CPUCount:         pointer.Ptr(uint64(10)),
					SchedulerPeriod:  pointer.Ptr(uint64(100)),
					SchedulerQuota:   pointer.Ptr(uint64(50)),
					PSISome: cgroups.PSIStats{
						Total: pointer.Ptr(uint64(96)),
					},
				},
				Memory: &cgroups.MemoryStats{
					UsageTotal:   pointer.Ptr(uint64(100)),
					KernelMemory: pointer.Ptr(uint64(40)),
					Limit:        pointer.Ptr(uint64(42000)),
					LowThreshold: pointer.Ptr(uint64(40000)),
					RSS:          pointer.Ptr(uint64(300)),
					Cache:        pointer.Ptr(uint64(200)),
					Swap:         pointer.Ptr(uint64(0)),
					SwapLimit:    pointer.Ptr(uint64(500)),
					OOMEvents:    pointer.Ptr(uint64(10)),
					PSISome: cgroups.PSIStats{
						Total: pointer.Ptr(uint64(97)),
					},
				},
				IOStats: &cgroups.IOStats{
					ReadBytes:       pointer.Ptr(uint64(100)),
					WriteBytes:      pointer.Ptr(uint64(200)),
					ReadOperations:  pointer.Ptr(uint64(10)),
					WriteOperations: pointer.Ptr(uint64(20)),
					PSISome: cgroups.PSIStats{
						Total: pointer.Ptr(uint64(98)),
					},
					// Device will be ignored as no matching device name
					Devices: map[string]cgroups.DeviceIOStats{
						"foo": {
							ReadBytes:       pointer.Ptr(uint64(100)),
							WriteBytes:      pointer.Ptr(uint64(200)),
							ReadOperations:  pointer.Ptr(uint64(10)),
							WriteOperations: pointer.Ptr(uint64(20)),
						},
					},
				},
				PIDStats: &cgroups.PIDStats{
					HierarchicalThreadCount: pointer.Ptr(uint64(10)),
					HierarchicalThreadLimit: pointer.Ptr(uint64(20)),
				},
				PIDs: []int{4, 2},
			},
			wantStats: &provider.ContainerStats{
				CPU: &provider.ContainerCPUStats{
					Total:            pointer.Ptr(100.0),
					System:           pointer.Ptr(200.0),
					User:             pointer.Ptr(300.0),
					Shares:           pointer.Ptr(400.0),
					Limit:            pointer.Ptr(50.0),
					ElapsedPeriods:   pointer.Ptr(500.0),
					ThrottledPeriods: pointer.Ptr(0.0),
					ThrottledTime:    pointer.Ptr(100.0),
					PartialStallTime: pointer.Ptr(96000.0),
				},
				Memory: &provider.ContainerMemStats{
					UsageTotal:       pointer.Ptr(100.0),
					KernelMemory:     pointer.Ptr(40.0),
					Limit:            pointer.Ptr(42000.0),
					Softlimit:        pointer.Ptr(40000.0),
					RSS:              pointer.Ptr(300.0),
					Cache:            pointer.Ptr(200.0),
					Swap:             pointer.Ptr(0.0),
					SwapLimit:        pointer.Ptr(500.0),
					OOMEvents:        pointer.Ptr(10.0),
					PartialStallTime: pointer.Ptr(97000.0),
				},
				IO: &provider.ContainerIOStats{
					ReadBytes:        pointer.Ptr(100.0),
					WriteBytes:       pointer.Ptr(200.0),
					ReadOperations:   pointer.Ptr(10.0),
					WriteOperations:  pointer.Ptr(20.0),
					PartialStallTime: pointer.Ptr(98000.0),
				},
				PID: &provider.ContainerPIDStats{
					PIDs:        []int{4, 2},
					ThreadCount: pointer.Ptr(10.0),
					ThreadLimit: pointer.Ptr(20.0),
				},
			},
			wantErr: false,
		},
		{
			name: "structs with partial errors",
			cg: &cgroups.MockCgroup{
				CPU: &cgroups.CPUStats{
					Total:            pointer.Ptr(uint64(100)),
					System:           pointer.Ptr(uint64(200)),
					User:             pointer.Ptr(uint64(300)),
					Shares:           pointer.Ptr(uint64(400)),
					ElapsedPeriods:   pointer.Ptr(uint64(500)),
					ThrottledPeriods: pointer.Ptr(uint64(0)),
					ThrottledTime:    pointer.Ptr(uint64(100)),
					CPUCount:         pointer.Ptr(uint64(10)),
					SchedulerPeriod:  pointer.Ptr(uint64(100)),
					SchedulerQuota:   pointer.Ptr(uint64(50)),
				},
				Memory: &cgroups.MemoryStats{
					UsageTotal:   pointer.Ptr(uint64(100)),
					KernelMemory: pointer.Ptr(uint64(40)),
					Limit:        pointer.Ptr(uint64(42000)),
					LowThreshold: pointer.Ptr(uint64(40000)),
					RSS:          pointer.Ptr(uint64(300)),
					Cache:        pointer.Ptr(uint64(200)),
					Swap:         pointer.Ptr(uint64(0)),
					SwapLimit:    pointer.Ptr(uint64(500)),
					OOMEvents:    pointer.Ptr(uint64(10)),
				},
				IOStats: &cgroups.IOStats{
					ReadBytes:       pointer.Ptr(uint64(100)),
					WriteBytes:      pointer.Ptr(uint64(200)),
					ReadOperations:  pointer.Ptr(uint64(10)),
					WriteOperations: pointer.Ptr(uint64(20)),
					// Device will be ignored as no matching device name
					Devices: map[string]cgroups.DeviceIOStats{
						"foo": {
							ReadBytes:       pointer.Ptr(uint64(100)),
							WriteBytes:      pointer.Ptr(uint64(200)),
							ReadOperations:  pointer.Ptr(uint64(10)),
							WriteOperations: pointer.Ptr(uint64(20)),
						},
					},
				},
				PIDStats:  nil,
				PIDError:  errors.New("unable to get PIDs"),
				PIDsError: errors.New("unable to get PIDs"),
			},
			wantStats: &provider.ContainerStats{
				CPU: &provider.ContainerCPUStats{
					Total:            pointer.Ptr(100.0),
					System:           pointer.Ptr(200.0),
					User:             pointer.Ptr(300.0),
					Shares:           pointer.Ptr(400.0),
					Limit:            pointer.Ptr(50.0),
					ElapsedPeriods:   pointer.Ptr(500.0),
					ThrottledPeriods: pointer.Ptr(0.0),
					ThrottledTime:    pointer.Ptr(100.0),
				},
				Memory: &provider.ContainerMemStats{
					UsageTotal:   pointer.Ptr(100.0),
					KernelMemory: pointer.Ptr(40.0),
					Limit:        pointer.Ptr(42000.0),
					Softlimit:    pointer.Ptr(40000.0),
					RSS:          pointer.Ptr(300.0),
					Cache:        pointer.Ptr(200.0),
					Swap:         pointer.Ptr(0.0),
					SwapLimit:    pointer.Ptr(500.0),
					OOMEvents:    pointer.Ptr(10.0),
				},
				IO: &provider.ContainerIOStats{
					ReadBytes:       pointer.Ptr(100.0),
					WriteBytes:      pointer.Ptr(200.0),
					ReadOperations:  pointer.Ptr(10.0),
					WriteOperations: pointer.Ptr(20.0),
				},
			},
			wantErr: false,
		},
		{
			name: "limit cpu count no quota",
			cg: &cgroups.MockCgroup{
				CPU: &cgroups.CPUStats{
					CPUCount: pointer.Ptr(uint64(10)),
				},
			},
			wantStats: &provider.ContainerStats{
				CPU: &provider.ContainerCPUStats{
					Limit: pointer.Ptr(1000.0),
				},
				PID:    &provider.ContainerPIDStats{},
				Memory: &provider.ContainerMemStats{},
				IO:     &provider.ContainerIOStats{},
			},
			wantErr: false,
		},
		{
			name: "limit no cpu count, no quota",
			cg: &cgroups.MockCgroup{
				CPU: &cgroups.CPUStats{},
			},
			wantStats: &provider.ContainerStats{
				CPU: &provider.ContainerCPUStats{
					Limit: pointer.Ptr(float64(utilsystem.HostCPUCount()) * 100),
				},
				PID:    &provider.ContainerPIDStats{},
				Memory: &provider.ContainerMemStats{},
				IO:     &provider.ContainerIOStats{},
			},
			wantErr: false,
		},
		{
			name: "limit cpu count on parent",
			cg: &cgroups.MockCgroup{
				CPU: &cgroups.CPUStats{
					CPUCount: pointer.Ptr(uint64(utilsystem.HostCPUCount())),
				},
				Parent: &cgroups.MockCgroup{
					CPU: &cgroups.CPUStats{
						CPUCount:        pointer.Ptr(uint64(utilsystem.HostCPUCount())),
						SchedulerPeriod: pointer.Ptr(uint64(100)),
						SchedulerQuota:  pointer.Ptr(uint64(10)),
					},
				},
			},
			wantStats: &provider.ContainerStats{
				CPU: &provider.ContainerCPUStats{
					Limit: pointer.Ptr(10.0),
				},
				PID:    &provider.ContainerPIDStats{},
				Memory: &provider.ContainerMemStats{},
				IO:     &provider.ContainerIOStats{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &systemCollector{}
			got, err := c.buildContainerMetrics(tt.cg, 0)
			if tt.wantErr {
				assert.NotNil(t, err)
			} else {
				assert.NoError(t, err)
			}
			if tt.wantStats != nil {
				tt.wantStats.Timestamp = got.Timestamp
			}
			assert.Empty(t, cmp.Diff(tt.wantStats, got))
		})
	}
}
