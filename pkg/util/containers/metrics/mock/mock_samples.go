// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mock

import (
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

// GetFullSampleContainerEntry returns a sample MockContainerEntry with
func GetFullSampleContainerEntry() ContainerEntry {
	return ContainerEntry{
		Error: nil,
		NetworkStats: &metrics.ContainerNetworkStats{
			BytesSent:   pointer.Float64Ptr(42),
			BytesRcvd:   pointer.Float64Ptr(43),
			PacketsSent: pointer.Float64Ptr(420),
			PacketsRcvd: pointer.Float64Ptr(421),
			Interfaces: map[string]metrics.InterfaceNetStats{
				"eth42": {
					BytesSent:   pointer.Float64Ptr(42),
					BytesRcvd:   pointer.Float64Ptr(43),
					PacketsSent: pointer.Float64Ptr(420),
					PacketsRcvd: pointer.Float64Ptr(421),
				},
			},
		},
		ContainerStats: &metrics.ContainerStats{
			CPU: &metrics.ContainerCPUStats{
				Total:            pointer.Float64Ptr(100),
				System:           pointer.Float64Ptr(200),
				User:             pointer.Float64Ptr(300),
				Shares:           pointer.Float64Ptr(400),
				Limit:            pointer.Float64Ptr(50),
				ElapsedPeriods:   pointer.Float64Ptr(500),
				ThrottledPeriods: pointer.Float64Ptr(0),
				ThrottledTime:    pointer.Float64Ptr(100),
			},
			Memory: &metrics.ContainerMemStats{
				UsageTotal:   pointer.Float64Ptr(42000),
				KernelMemory: pointer.Float64Ptr(40),
				Limit:        pointer.Float64Ptr(42000),
				SwapLimit:    pointer.Float64Ptr(500),
				Softlimit:    pointer.Float64Ptr(40000),
				RSS:          pointer.Float64Ptr(300),
				Cache:        pointer.Float64Ptr(200),
				Swap:         pointer.Float64Ptr(0),
				OOMEvents:    pointer.Float64Ptr(10),
			},
			IO: &metrics.ContainerIOStats{
				Devices: map[string]metrics.DeviceIOStats{
					"/dev/foo": {
						ReadBytes:       pointer.Float64Ptr(100),
						WriteBytes:      pointer.Float64Ptr(200),
						ReadOperations:  pointer.Float64Ptr(10),
						WriteOperations: pointer.Float64Ptr(20),
					},
					"/dev/bar": {
						ReadBytes:       pointer.Float64Ptr(100),
						WriteBytes:      pointer.Float64Ptr(200),
						ReadOperations:  pointer.Float64Ptr(10),
						WriteOperations: pointer.Float64Ptr(20),
					},
				},
				ReadBytes:       pointer.Float64Ptr(200),
				WriteBytes:      pointer.Float64Ptr(400),
				ReadOperations:  pointer.Float64Ptr(20),
				WriteOperations: pointer.Float64Ptr(40),
			},
			PID: &metrics.ContainerPIDStats{
				PIDs:        []int{4, 2},
				ThreadCount: pointer.Float64Ptr(10),
				ThreadLimit: pointer.Float64Ptr(20),
			},
		},
		OpenFiles: pointer.UInt64Ptr(200),
	}
}
