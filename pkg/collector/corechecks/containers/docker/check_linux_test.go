// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker && linux
// +build docker,linux

package docker

import (
	"testing"

	dockerTypes "github.com/docker/docker/api/types"
	dockerNetworkTypes "github.com/docker/docker/api/types/network"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/generic"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/system"
)

func TestDockerNetworkExtension(t *testing.T) {
	routeForPID := map[int][]system.NetworkRoute{
		// Host network (Kubernetes)
		100: {
			{
				Interface: "eth0",
				Subnet:    0x00000000,
				Mask:      0x00000000,
				Gateway:   0x0180000A,
			},
			{
				Interface: "eth0",
				Subnet:    0x0180000A,
				Mask:      0xFFFFFFFF,
				Gateway:   0x00000000,
			},
			{
				Interface: "cbr0",
				Subnet:    0x0000A00A,
				Mask:      0x00FFFFFF,
				Gateway:   0x00000000,
			},
			{
				Interface: "docker0",
				Subnet:    0x007BFEA9,
				Mask:      0x00FFFFFF,
				Gateway:   0x00000000,
			},
		},
		// Container in Kubernetes
		101: {
			{
				Interface: "eth0",
				Subnet:    0x00000000,
				Mask:      0x00000000,
				Gateway:   0x0100A00A,
			},
			{
				Interface: "eth0",
				Subnet:    0x0000A00A,
				Mask:      0x00FFFFFF,
				Gateway:   0x00000000,
			},
		},
		// Container in Docker
		200: {
			{
				Interface: "eth1",
				Subnet:    0x00000000,
				Mask:      0x00000000,
				Gateway:   0x010011AC,
			},
			{
				Interface: "eth1",
				Subnet:    0x000011AC,
				Mask:      0x0000FFFF,
				Gateway:   0x00000000,
			},
			{
				Interface: "eth0",
				Subnet:    0x000012AC,
				Mask:      0x0000FFFF,
				Gateway:   0x00000000,
			},
		},
	}

	getRoutesFunc = func(procPath string, pid int) ([]system.NetworkRoute, error) {
		return routeForPID[pid], nil
	}

	mockSender := mocksender.NewMockSender("docker-network-extension")
	mockSender.SetupAcceptAll()

	mockCollector := metrics.NewMockCollector("testCollector")

	// Test setup:
	// container1 is host network in Kubernetes - linked to PID 100
	// container2 is normal container in Kubernetes (no network config) - linked to container3 and PID 101
	// container3 is a pause container in Kubernetes (owns the network config) - linked to nothing
	// container4 is a normal docker container connected to 2 networks 0 linked to PID 200
	container1 := createContainerMeta("docker", "kube-host-network")
	mockCollector.SetContainerEntry(container1.ID, metrics.MockContainerEntry{
		ContainerStats: provider.ContainerStats{
			PID: &provider.ContainerPIDStats{
				PIDs: []int{100},
			},
		},
		NetworkStats: metrics.ContainerNetworkStats{
			Interfaces: map[string]metrics.InterfaceNetStats{
				"eth0": {
					BytesSent:   util.Float64Ptr(1),
					BytesRcvd:   util.Float64Ptr(1),
					PacketsSent: util.Float64Ptr(1),
					PacketsRcvd: util.Float64Ptr(1),
				},
				"docker0": {
					BytesSent:   util.Float64Ptr(2),
					BytesRcvd:   util.Float64Ptr(2),
					PacketsSent: util.Float64Ptr(2),
					PacketsRcvd: util.Float64Ptr(2),
				},
				"cbr0": {
					BytesSent:   util.Float64Ptr(3),
					BytesRcvd:   util.Float64Ptr(3),
					PacketsSent: util.Float64Ptr(3),
					PacketsRcvd: util.Float64Ptr(3),
				},
				"vethc71e3170": {
					BytesSent:   util.Float64Ptr(4),
					BytesRcvd:   util.Float64Ptr(4),
					PacketsSent: util.Float64Ptr(4),
					PacketsRcvd: util.Float64Ptr(4),
				},
			},
		},
	})
	container1RawDocker := dockerTypes.Container{
		ID:    "kube-host-network",
		State: containers.ContainerRunningState,
		HostConfig: struct {
			NetworkMode string "json:\",omitempty\""
		}{NetworkMode: "host"},
		NetworkSettings: &dockerTypes.SummaryNetworkSettings{
			Networks: map[string]*dockerNetworkTypes.EndpointSettings{
				"host": {
					NetworkID:  "someid",
					EndpointID: "someid",
				},
			},
		},
	}

	container2 := createContainerMeta("docker", "kube-app")
	mockCollector.SetContainerEntry(container2.ID, metrics.MockContainerEntry{
		ContainerStats: provider.ContainerStats{
			PID: &provider.ContainerPIDStats{
				PIDs: []int{101},
			},
		},
		NetworkStats: metrics.ContainerNetworkStats{
			Interfaces: map[string]metrics.InterfaceNetStats{
				"eth0": {
					BytesSent:   util.Float64Ptr(5),
					BytesRcvd:   util.Float64Ptr(5),
					PacketsSent: util.Float64Ptr(5),
					PacketsRcvd: util.Float64Ptr(5),
				},
			},
		},
	})
	container2RawDocker := dockerTypes.Container{
		ID:    "kube-app",
		State: containers.ContainerRunningState,
		HostConfig: struct {
			NetworkMode string "json:\",omitempty\""
		}{NetworkMode: "container:kube-app-pause"},
		NetworkSettings: &dockerTypes.SummaryNetworkSettings{
			Networks: map[string]*dockerNetworkTypes.EndpointSettings{},
		},
	}

	// Container3 is only raw as it's excluded (pause container)
	container3RawDocker := dockerTypes.Container{
		ID:    "kube-app-pause",
		State: containers.ContainerRunningState,
		HostConfig: struct {
			NetworkMode string "json:\",omitempty\""
		}{NetworkMode: "none"},
		NetworkSettings: &dockerTypes.SummaryNetworkSettings{
			Networks: map[string]*dockerNetworkTypes.EndpointSettings{
				"none": {
					NetworkID:  "someid",
					EndpointID: "someid",
				},
			},
		},
	}

	container4 := createContainerMeta("docker", "docker-app")
	mockCollector.SetContainerEntry(container4.ID, metrics.MockContainerEntry{
		ContainerStats: provider.ContainerStats{
			PID: &provider.ContainerPIDStats{
				PIDs: []int{200},
			},
		},
		NetworkStats: metrics.ContainerNetworkStats{
			Interfaces: map[string]metrics.InterfaceNetStats{
				"eth0": {
					BytesSent:   util.Float64Ptr(6),
					BytesRcvd:   util.Float64Ptr(6),
					PacketsSent: util.Float64Ptr(6),
					PacketsRcvd: util.Float64Ptr(6),
				},
				"eth1": {
					BytesSent:   util.Float64Ptr(7),
					BytesRcvd:   util.Float64Ptr(7),
					PacketsSent: util.Float64Ptr(7),
					PacketsRcvd: util.Float64Ptr(7),
				},
			},
		},
	})
	container4RawDocker := dockerTypes.Container{
		ID:    "docker-app",
		State: containers.ContainerRunningState,
		HostConfig: struct {
			NetworkMode string "json:\",omitempty\""
		}{NetworkMode: "ubuntu_default"},
		NetworkSettings: &dockerTypes.SummaryNetworkSettings{
			Networks: map[string]*dockerNetworkTypes.EndpointSettings{
				"ubuntu_default": {
					IPAddress: "172.18.0.2",
				},
				"bridge": {
					IPAddress: "172.17.0.2",
				},
			},
		},
	}

	// Running them through the dockerNetworkExtension
	tags := []string{"foo:bar"}
	dockerNetworkExtension := dockerNetworkExtension{}

	// Running the extension part
	dockerNetworkExtension.PreProcess(generic.MockSendMetric, mockSender)
	dockerNetworkExtension.Process(tags, container1, mockCollector, 0)
	dockerNetworkExtension.Process(tags, container2, mockCollector, 0)
	dockerNetworkExtension.Process(tags, container4, mockCollector, 0)
	dockerNetworkExtension.PostProcess()

	// Running the custom part
	dockerNetworkExtension.preRun()
	dockerNetworkExtension.processContainer(container1RawDocker)
	dockerNetworkExtension.processContainer(container2RawDocker)
	dockerNetworkExtension.processContainer(container3RawDocker)
	dockerNetworkExtension.processContainer(container4RawDocker)
	dockerNetworkExtension.postRun()

	// Checking results
	mockSender.AssertNumberOfCalls(t, "Rate", 14)

	// Container 1
	mockSender.AssertMetric(t, "Rate", "docker.net.bytes_rcvd", 1, "", []string{"foo:bar", "docker_network:eth0"})
	mockSender.AssertMetric(t, "Rate", "docker.net.bytes_sent", 1, "", []string{"foo:bar", "docker_network:eth0"})
	mockSender.AssertMetric(t, "Rate", "docker.net.bytes_rcvd", 2, "", []string{"foo:bar", "docker_network:docker0"})
	mockSender.AssertMetric(t, "Rate", "docker.net.bytes_sent", 2, "", []string{"foo:bar", "docker_network:docker0"})
	mockSender.AssertMetric(t, "Rate", "docker.net.bytes_rcvd", 3, "", []string{"foo:bar", "docker_network:cbr0"})
	mockSender.AssertMetric(t, "Rate", "docker.net.bytes_sent", 3, "", []string{"foo:bar", "docker_network:cbr0"})
	mockSender.AssertMetric(t, "Rate", "docker.net.bytes_rcvd", 4, "", []string{"foo:bar", "docker_network:vethc71e3170"})
	mockSender.AssertMetric(t, "Rate", "docker.net.bytes_sent", 4, "", []string{"foo:bar", "docker_network:vethc71e3170"})

	// Container 2
	mockSender.AssertMetric(t, "Rate", "docker.net.bytes_rcvd", 5, "", []string{"foo:bar", "docker_network:bridge"})
	mockSender.AssertMetric(t, "Rate", "docker.net.bytes_sent", 5, "", []string{"foo:bar", "docker_network:bridge"})

	// Container 4
	mockSender.AssertMetric(t, "Rate", "docker.net.bytes_rcvd", 6, "", []string{"foo:bar", "docker_network:ubuntu_default"})
	mockSender.AssertMetric(t, "Rate", "docker.net.bytes_sent", 6, "", []string{"foo:bar", "docker_network:ubuntu_default"})
	mockSender.AssertMetric(t, "Rate", "docker.net.bytes_rcvd", 7, "", []string{"foo:bar", "docker_network:bridge"})
	mockSender.AssertMetric(t, "Rate", "docker.net.bytes_sent", 7, "", []string{"foo:bar", "docker_network:bridge"})
}
