// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import "github.com/DataDog/datadog-agent/pkg/util"

// GetFullSampleContainerEntry returns a sample MockContainerEntry with
func GetFullSampleContainerEntry() MockContainerEntry {
	return MockContainerEntry{
		Error: nil,
		NetworkStats: ContainerNetworkStats{
			BytesSent:   util.Float64Ptr(42),
			BytesRcvd:   util.Float64Ptr(43),
			PacketsSent: util.Float64Ptr(420),
			PacketsRcvd: util.Float64Ptr(421),
			Interfaces: map[string]InterfaceNetStats{
				"eth42": {
					BytesSent:   util.Float64Ptr(42),
					BytesRcvd:   util.Float64Ptr(43),
					PacketsSent: util.Float64Ptr(420),
					PacketsRcvd: util.Float64Ptr(421),
				},
			},
		},
		ContainerStats: ContainerStats{
			CPU: &ContainerCPUStats{
				Total:            util.Float64Ptr(100),
				System:           util.Float64Ptr(200),
				User:             util.Float64Ptr(300),
				Shares:           util.Float64Ptr(400),
				Limit:            util.Float64Ptr(50),
				ElapsedPeriods:   util.Float64Ptr(500),
				ThrottledPeriods: util.Float64Ptr(0),
				ThrottledTime:    util.Float64Ptr(100),
			},
			Memory: &ContainerMemStats{
				UsageTotal:   util.Float64Ptr(42000),
				KernelMemory: util.Float64Ptr(40),
				Limit:        util.Float64Ptr(42000),
				Softlimit:    util.Float64Ptr(40000),
				RSS:          util.Float64Ptr(300),
				Cache:        util.Float64Ptr(200),
				Swap:         util.Float64Ptr(0),
				OOMEvents:    util.Float64Ptr(10),
			},
			IO: &ContainerIOStats{
				Devices: map[string]DeviceIOStats{
					"/dev/foo": {
						ReadBytes:       util.Float64Ptr(100),
						WriteBytes:      util.Float64Ptr(200),
						ReadOperations:  util.Float64Ptr(10),
						WriteOperations: util.Float64Ptr(20),
					},
					"/dev/bar": {
						ReadBytes:       util.Float64Ptr(100),
						WriteBytes:      util.Float64Ptr(200),
						ReadOperations:  util.Float64Ptr(10),
						WriteOperations: util.Float64Ptr(20),
					},
				},
				ReadBytes:       util.Float64Ptr(200),
				WriteBytes:      util.Float64Ptr(400),
				ReadOperations:  util.Float64Ptr(20),
				WriteOperations: util.Float64Ptr(40),
			},
			PID: &ContainerPIDStats{
				PIDs:        []int{4, 2},
				ThreadCount: util.Float64Ptr(10),
				ThreadLimit: util.Float64Ptr(20),
				OpenFiles:   util.Float64Ptr(200),
			},
		},
	}
}
