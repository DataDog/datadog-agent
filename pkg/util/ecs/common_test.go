// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build docker

package ecs

import (
	"net"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	v2 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v2"
	"github.com/stretchr/testify/assert"
)

func TestConvertMetaV2Container(t *testing.T) {
	container := v2.Container{
		CreatedAt:  "2018-02-01T20:55:10.554941919Z",
		DockerID:   "43481a6ce4842eec8fe72fc28500c6b52edcc0917f105b83379f88cac1ff3946",
		DockerName: "ecs-nginx-5-nginx-curl-ccccb9f49db0dfe0d901",
		Image:      "nrdlngr/nginx-curl",
		ImageID:    "sha256:2e00ae64383cfc865ba0a2ba37f61b50a120d2d9378559dcd458dc0de47bc165",
		StartedAt:  "2018-02-01T20:55:11.064236631Z",
		Limits: map[string]uint64{
			"cpu":    0,
			"memory": 0,
		},
	}
	expected := &containers.Container{
		Created:     1517518510,
		EntityID:    "container_id://43481a6ce4842eec8fe72fc28500c6b52edcc0917f105b83379f88cac1ff3946",
		ID:          "43481a6ce4842eec8fe72fc28500c6b52edcc0917f105b83379f88cac1ff3946",
		Image:       "nrdlngr/nginx-curl",
		ImageID:     "sha256:2e00ae64383cfc865ba0a2ba37f61b50a120d2d9378559dcd458dc0de47bc165",
		Name:        "ecs-nginx-5-nginx-curl-ccccb9f49db0dfe0d901",
		StartedAt:   1517518511,
		Type:        "ECS",
		AddressList: []containers.NetworkAddress{},
	}
	expected.CPULimit = 100

	assert.Equal(t, expected, convertMetaV2Container(container))
}

func TestConvertMetaV2ContainerStats(t *testing.T) {
	stats := &v2.ContainerStats{
		CPU: v2.CPUStats{
			System: 3951680000000,
			Usage: v2.CPUUsage{
				Kernelmode: 2260000000,
				Total:      9743590394,
				Usermode:   7450000000,
			},
		},
		Memory: v2.MemStats{
			Details: v2.DetailedMem{
				RSS:     1564672,
				Cache:   65499136,
				PgFault: 430478,
			},
			Limit:    268435456,
			MaxUsage: 139751424,
			Usage:    77254656,
		},
		IO: v2.IOStats{
			BytesPerDeviceAndKind: []v2.OPStat{
				{
					Kind:  "Read",
					Major: 259,
					Minor: 0,
					Value: 12288,
				},
				{
					Kind:  "Write",
					Major: 259,
					Minor: 0,
					Value: 144908288,
				},
				{
					Kind:  "Sync",
					Major: 259,
					Minor: 0,
					Value: 8122368,
				},
				{
					Kind:  "Async",
					Major: 259,
					Minor: 0,
					Value: 136798208,
				},
				{
					Kind:  "Total",
					Major: 259,
					Minor: 0,
					Value: 144920576,
				},
			},
			OPPerDeviceAndKind: []v2.OPStat{
				{
					Kind:  "Read",
					Major: 259,
					Minor: 0,
					Value: 3,
				},
				{
					Kind:  "Write",
					Major: 259,
					Minor: 0,
					Value: 1618,
				},
				{
					Kind:  "Sync",
					Major: 259,
					Minor: 0,
					Value: 514,
				},
				{
					Kind:  "Async",
					Major: 259,
					Minor: 0,
					Value: 1107,
				},
				{
					Kind:  "Total",
					Major: 259,
					Minor: 0,
					Value: 1621,
				},
			},
			ReadBytes:  1024,
			WriteBytes: 256,
		},
		Network: v2.NetStats{},
	}

	expectedCPU := &metrics.ContainerCPUStats{
		User:        7450000000,
		System:      2260000000,
		SystemUsage: 3951680000000,
	}
	expectedMem := &metrics.ContainerMemStats{
		Cache:           65499136,
		MemUsageInBytes: 77254656,
		Pgfault:         430478,
		RSS:             1564672,
	}
	expectedIO := &metrics.ContainerIOStats{
		ReadBytes:  1024,
		WriteBytes: 256,
	}

	containerMetrics, memLimit := convertMetaV2ContainerStats(stats)

	assert.Equal(t, expectedCPU, containerMetrics.CPU)
	assert.Equal(t, expectedMem, containerMetrics.Memory)
	assert.Equal(t, expectedIO, containerMetrics.IO)
	assert.Equal(t, uint64(268435456), memLimit)
}

func TestParseContainerNetworkAddresses(t *testing.T) {
	ports := []v2.Port{
		{
			ContainerPort: 80,
			Protocol:      "tcp",
		},
		{
			ContainerPort: 7000,
			Protocol:      "udp",
		},
	}
	networks := []v2.Network{
		{
			NetworkMode:   "awsvpc",
			IPv4Addresses: []string{"10.0.2.106"},
		},
		{
			NetworkMode:   "awsvpc",
			IPv4Addresses: []string{"10.0.2.107"},
		},
	}
	expectedOutput := []containers.NetworkAddress{
		{
			IP:       net.ParseIP("10.0.2.106"),
			Port:     80,
			Protocol: "tcp",
		},
		{
			IP:       net.ParseIP("10.0.2.106"),
			Port:     7000,
			Protocol: "udp",
		},
		{
			IP:       net.ParseIP("10.0.2.107"),
			Port:     80,
			Protocol: "tcp",
		},
		{
			IP:       net.ParseIP("10.0.2.107"),
			Port:     7000,
			Protocol: "udp",
		},
	}
	result := parseContainerNetworkAddresses(ports, networks, "mycontainer")
	assert.Equal(t, expectedOutput, result)
}
