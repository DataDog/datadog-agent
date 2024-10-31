// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker && linux

package docker

import (
	"testing"

	dockerTypes "github.com/docker/docker/api/types"
	dockerNetworkTypes "github.com/docker/docker/api/types/network"

	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/noopimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/generic"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/mock"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
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

	getRoutesFunc = func(_ string, pid int) ([]system.NetworkRoute, error) {
		return routeForPID[pid], nil
	}

	mockSender := mocksender.NewMockSender("docker-network-extension")
	mockSender.SetupAcceptAll()

	mockCollector := mock.NewCollector("testCollector")

	// Test setup:
	// container1 is host network in Kubernetes - linked to PID 100
	// container2 is normal container in Kubernetes (no network config) - linked to container3 and PID 101
	// container3 is a pause container in Kubernetes (owns the network config) - linked to nothing
	// container4 is a normal docker container connected to 2 networks 0 linked to PID 200
	container1 := generic.CreateContainerMeta("docker", "kube-host-network")
	mockCollector.SetContainerEntry(container1.ID, mock.ContainerEntry{
		PIDs: []int{100},
		NetworkStats: &metrics.ContainerNetworkStats{
			Interfaces: map[string]metrics.InterfaceNetStats{
				"eth0": {
					BytesSent:   pointer.Ptr(1.0),
					BytesRcvd:   pointer.Ptr(1.0),
					PacketsSent: pointer.Ptr(1.0),
					PacketsRcvd: pointer.Ptr(1.0),
				},
				"docker0": {
					BytesSent:   pointer.Ptr(2.0),
					BytesRcvd:   pointer.Ptr(2.0),
					PacketsSent: pointer.Ptr(2.0),
					PacketsRcvd: pointer.Ptr(2.0),
				},
				"cbr0": {
					BytesSent:   pointer.Ptr(3.0),
					BytesRcvd:   pointer.Ptr(3.0),
					PacketsSent: pointer.Ptr(3.0),
					PacketsRcvd: pointer.Ptr(3.0),
				},
				"vethc71e3170": {
					BytesSent:   pointer.Ptr(4.0),
					BytesRcvd:   pointer.Ptr(4.0),
					PacketsSent: pointer.Ptr(4.0),
					PacketsRcvd: pointer.Ptr(4.0),
				},
			},
		},
	})
	container1RawDocker := dockerTypes.Container{
		ID:    "kube-host-network",
		State: string(workloadmeta.ContainerStatusRunning),
		HostConfig: struct {
			NetworkMode string            `json:",omitempty"`
			Annotations map[string]string `json:",omitempty"`
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

	container2 := generic.CreateContainerMeta("docker", "kube-app")
	mockCollector.SetContainerEntry(container2.ID, mock.ContainerEntry{
		PIDs: []int{101},
		NetworkStats: &metrics.ContainerNetworkStats{
			Interfaces: map[string]metrics.InterfaceNetStats{
				"eth0": {
					BytesSent:   pointer.Ptr(5.0),
					BytesRcvd:   pointer.Ptr(5.0),
					PacketsSent: pointer.Ptr(5.0),
					PacketsRcvd: pointer.Ptr(5.0),
				},
			},
		},
	})
	container2RawDocker := dockerTypes.Container{
		ID:    "kube-app",
		State: string(workloadmeta.ContainerStatusRunning),
		HostConfig: struct {
			NetworkMode string            `json:",omitempty"`
			Annotations map[string]string `json:",omitempty"`
		}{NetworkMode: "container:kube-app-pause"},
		NetworkSettings: &dockerTypes.SummaryNetworkSettings{
			Networks: map[string]*dockerNetworkTypes.EndpointSettings{},
		},
	}

	// Container3 is only raw as it's excluded (pause container)
	container3RawDocker := dockerTypes.Container{
		ID:    "kube-app-pause",
		State: string(workloadmeta.ContainerStatusRunning),
		HostConfig: struct {
			NetworkMode string            `json:",omitempty"`
			Annotations map[string]string `json:",omitempty"`
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

	container4 := generic.CreateContainerMeta("docker", "docker-app")
	mockCollector.SetContainerEntry(container4.ID, mock.ContainerEntry{
		PIDs: []int{200},
		NetworkStats: &metrics.ContainerNetworkStats{
			Interfaces: map[string]metrics.InterfaceNetStats{
				"eth0": {
					BytesSent:   pointer.Ptr(6.0),
					BytesRcvd:   pointer.Ptr(6.0),
					PacketsSent: pointer.Ptr(6.0),
					PacketsRcvd: pointer.Ptr(6.0),
				},
				"eth1": {
					BytesSent:   pointer.Ptr(7.0),
					BytesRcvd:   pointer.Ptr(7.0),
					PacketsSent: pointer.Ptr(7.0),
					PacketsRcvd: pointer.Ptr(7.0),
				},
			},
		},
	})
	container4RawDocker := dockerTypes.Container{
		ID:    "docker-app",
		State: string(workloadmeta.ContainerStatusRunning),
		HostConfig: struct {
			NetworkMode string            `json:",omitempty"`
			Annotations map[string]string `json:",omitempty"`
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
	dockerNetworkExtension.PostProcess(nooptagger.NewTaggerClient())

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

//nolint:revive // TODO(CINT) Fix revive linter
func TestNetworkCustomOnFailure(t *testing.T) {
	// Make sure we don't panic if generic part fails
	networkExt := dockerNetworkExtension{procPath: "/proc"}

	networkExt.preRun()
	networkExt.processContainer(dockerTypes.Container{
		ID:      "e2d5394a5321d4a59497f53552a0131b2aafe64faba37f4738e78c531289fc45",
		Names:   []string{"agent"},
		Image:   "datadog/agent",
		ImageID: "sha256:7e813d42985b2e5a0269f868aaf238ffc952a877fba964f55aa1ff35fd0bf5f6",
		Labels: map[string]string{
			"io.kubernetes.pod.namespace": "kubens",
		},
		State:      string(workloadmeta.ContainerStatusRunning),
		SizeRw:     100,
		SizeRootFs: 200,
	})
	networkExt.postRun()
}
