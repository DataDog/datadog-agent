// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker
// +build docker

package ecsfargate

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	v2 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v2"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestConvertEcsNetworkStats(t *testing.T) {
	testTimeStr := "2020-10-02T00:51:13.410254284Z"
	testTime, err := time.Parse(time.RFC3339Nano, testTimeStr)
	assert.NoError(t, err)

	type args struct {
		ecsStats *v2.ContainerStats
	}
	tests := []struct {
		name string
		args args
		want *provider.ContainerNetworkStats
	}{
		{
			name: "nominal case",
			args: args{
				ecsStats: &v2.ContainerStats{
					Timestamp: testTimeStr,
					Networks:  v2.NetStatsMap{"eth1": v2.NetStats{RxBytes: 2398415937, RxPackets: 1898631, TxBytes: 1259037719, TxPackets: 428002}},
				},
			},
			want: &provider.ContainerNetworkStats{
				Timestamp:   testTime,
				Interfaces:  map[string]provider.InterfaceNetStats{"eth1": {BytesRcvd: pointer.UIntToFloatPtr(2398415937), PacketsRcvd: pointer.UIntToFloatPtr(1898631), BytesSent: pointer.UIntToFloatPtr(1259037719), PacketsSent: pointer.UIntToFloatPtr(428002)}},
				BytesRcvd:   pointer.UIntToFloatPtr(2398415937),
				PacketsRcvd: pointer.UIntToFloatPtr(1898631),
				BytesSent:   pointer.UIntToFloatPtr(1259037719),
				PacketsSent: pointer.UIntToFloatPtr(428002),
			},
		},
		{
			name: "multiple interfaces",
			args: args{
				ecsStats: &v2.ContainerStats{
					Timestamp: testTimeStr,
					Networks: v2.NetStatsMap{
						"eth0": v2.NetStats{RxBytes: 2398415937, RxPackets: 1898631, TxBytes: 1259037719, TxPackets: 428002},
						"eth1": v2.NetStats{TxBytes: 2398415936, TxPackets: 1898630, RxBytes: 1259037718, RxPackets: 428001},
					},
				},
			},
			want: &provider.ContainerNetworkStats{
				Timestamp: testTime,
				Interfaces: map[string]provider.InterfaceNetStats{
					"eth0": {BytesRcvd: pointer.UIntToFloatPtr(2398415937), PacketsRcvd: pointer.UIntToFloatPtr(1898631), BytesSent: pointer.UIntToFloatPtr(1259037719), PacketsSent: pointer.UIntToFloatPtr(428002)},
					"eth1": {BytesSent: pointer.UIntToFloatPtr(2398415936), PacketsSent: pointer.UIntToFloatPtr(1898630), BytesRcvd: pointer.UIntToFloatPtr(1259037718), PacketsRcvd: pointer.UIntToFloatPtr(428001)},
				},
				BytesRcvd:   pointer.UIntToFloatPtr(3657453655),
				PacketsRcvd: pointer.UIntToFloatPtr(2326632),
				BytesSent:   pointer.UIntToFloatPtr(3657453655),
				PacketsSent: pointer.UIntToFloatPtr(2326632),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.EqualValues(t, tt.want, convertNetworkStats(tt.args.ecsStats))
		})
	}
}

func TestConvertEcsStats(t *testing.T) {
	testTimeStr := "2020-10-02T00:51:13.410254284Z"
	testTime, err := time.Parse(time.RFC3339Nano, testTimeStr)
	assert.NoError(t, err)

	type args struct {
		ecsStats *v2.ContainerStats
	}
	tests := []struct {
		name string
		args args
		want *provider.ContainerStats
	}{
		{
			name: "nominal case",
			args: args{
				ecsStats: &v2.ContainerStats{
					Timestamp: testTimeStr,
					CPU:       v2.CPUStats{Usage: v2.CPUUsage{Total: 1137691504, Kernelmode: 80000000, Usermode: 810000000}},
					Memory:    v2.MemStats{Usage: 6504448, Details: v2.DetailedMem{RSS: 4669440, Cache: 651264}},
					IO: v2.IOStats{
						BytesPerDeviceAndKind: []v2.OPStat{
							{
								Major: 202,
								Minor: 26368,
								Kind:  "Read",
								Value: 638976,
							},
							{
								Major: 202,
								Minor: 26368,
								Kind:  "Write",
								Value: 0,
							},
							{
								Major: 202,
								Minor: 26368,
								Kind:  "Sync",
								Value: 638976,
							},
							{
								Major: 202,
								Minor: 26368,
								Kind:  "Async",
								Value: 0,
							},
							{
								Major: 202,
								Minor: 26368,
								Kind:  "Total",
								Value: 638976,
							},
						},
						OPPerDeviceAndKind: []v2.OPStat{
							{
								Major: 202,
								Minor: 26368,
								Kind:  "Read",
								Value: 12,
							},
							{
								Major: 202,
								Minor: 26368,
								Kind:  "Write",
								Value: 0,
							},
							{
								Major: 202,
								Minor: 26368,
								Kind:  "Sync",
								Value: 12,
							},
							{
								Major: 202,
								Minor: 26368,
								Kind:  "Async",
								Value: 0,
							},
							{
								Major: 202,
								Minor: 26368,
								Kind:  "Total",
								Value: 12,
							},
						},
					},
				},
			},
			want: &provider.ContainerStats{
				Timestamp: testTime,
				CPU: &provider.ContainerCPUStats{
					Total:  pointer.UIntToFloatPtr(1137691504),
					System: pointer.UIntToFloatPtr(80000000),
					User:   pointer.UIntToFloatPtr(810000000),
				},
				Memory: &provider.ContainerMemStats{
					UsageTotal: pointer.UIntToFloatPtr(6504448),
					RSS:        pointer.UIntToFloatPtr(4669440),
					Cache:      pointer.UIntToFloatPtr(651264),
				},
				IO: &provider.ContainerIOStats{
					ReadBytes:       pointer.UIntToFloatPtr(638976),
					WriteBytes:      pointer.UIntToFloatPtr(0),
					ReadOperations:  pointer.UIntToFloatPtr(12),
					WriteOperations: pointer.UIntToFloatPtr(0),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertEcsStats(tt.args.ecsStats)
			assert.EqualValues(t, tt.want, got)
		})
	}
}

func TestFillFromSpec(t *testing.T) {
	testSpec := &v2.Task{
		Limits: map[string]float64{
			cpuKey:    0.5,
			memoryKey: 4096,
		},
	}

	// Nominal case, no limits found from stats
	containerStats := &provider.ContainerStats{
		CPU:    &provider.ContainerCPUStats{},
		Memory: &provider.ContainerMemStats{},
	}
	fillFromSpec(containerStats, testSpec)
	assert.EqualValues(t, &provider.ContainerStats{
		CPU: &provider.ContainerCPUStats{
			Limit: pointer.Float64Ptr(50),
		},
		Memory: &provider.ContainerMemStats{
			Limit: pointer.Float64Ptr(4294967296),
		},
	}, containerStats)

	// Test no memory override
	containerStats = &provider.ContainerStats{
		CPU: &provider.ContainerCPUStats{},
		Memory: &provider.ContainerMemStats{
			Limit: pointer.Float64Ptr(1024),
		},
	}
	fillFromSpec(containerStats, testSpec)
	assert.EqualValues(t, &provider.ContainerStats{
		CPU: &provider.ContainerCPUStats{
			Limit: pointer.Float64Ptr(50),
		},
		Memory: &provider.ContainerMemStats{
			Limit: pointer.Float64Ptr(1024),
		},
	}, containerStats)
}
