// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

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
			BytesSent:   pointer.Ptr(42.0),
			BytesRcvd:   pointer.Ptr(43.0),
			PacketsSent: pointer.Ptr(420.0),
			PacketsRcvd: pointer.Ptr(421.0),
			Interfaces: map[string]metrics.InterfaceNetStats{
				"eth42": {
					BytesSent:   pointer.Ptr(42.0),
					BytesRcvd:   pointer.Ptr(43.0),
					PacketsSent: pointer.Ptr(420.0),
					PacketsRcvd: pointer.Ptr(421.0),
				},
			},
		},
		ContainerStats: &metrics.ContainerStats{
			CPU: &metrics.ContainerCPUStats{
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
			Memory: &metrics.ContainerMemStats{
				UsageTotal:       pointer.Ptr(42000.0),
				KernelMemory:     pointer.Ptr(40.0),
				Limit:            pointer.Ptr(42000.0),
				SwapLimit:        pointer.Ptr(500.0),
				Softlimit:        pointer.Ptr(40000.0),
				RSS:              pointer.Ptr(300.0),
				WorkingSet:       pointer.Ptr(350.0),
				Cache:            pointer.Ptr(200.0),
				Swap:             pointer.Ptr(0.0),
				OOMEvents:        pointer.Ptr(10.0),
				PartialStallTime: pointer.Ptr(97000.0),
				Peak:             pointer.Ptr(50000.0),
			},
			IO: &metrics.ContainerIOStats{
				Devices: map[string]metrics.DeviceIOStats{
					"/dev/foo": {
						ReadBytes:       pointer.Ptr(100.0),
						WriteBytes:      pointer.Ptr(200.0),
						ReadOperations:  pointer.Ptr(10.0),
						WriteOperations: pointer.Ptr(20.0),
					},
					"/dev/bar": {
						ReadBytes:       pointer.Ptr(100.0),
						WriteBytes:      pointer.Ptr(200.0),
						ReadOperations:  pointer.Ptr(10.0),
						WriteOperations: pointer.Ptr(20.0),
					},
				},
				ReadBytes:        pointer.Ptr(200.0),
				WriteBytes:       pointer.Ptr(400.0),
				ReadOperations:   pointer.Ptr(20.0),
				WriteOperations:  pointer.Ptr(40.0),
				PartialStallTime: pointer.Ptr(98000.0),
			},
			PID: &metrics.ContainerPIDStats{
				ThreadCount: pointer.Ptr(10.0),
				ThreadLimit: pointer.Ptr(20.0),
			},
		},
		OpenFiles: pointer.Ptr(uint64(200)),
		PIDs:      []int{4, 2},
	}
}
