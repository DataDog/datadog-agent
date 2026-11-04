// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker && linux

package docker

import (
	"fmt"
	"net/netip"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/moby/moby/api/types/container"
	dockerNetworkTypes "github.com/moby/moby/api/types/network"

	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
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
		routes, found := routeForPID[pid]
		if !found {
			return nil, fmt.Errorf("unable to read routes for pid %d: %w", pid, os.ErrNotExist)
		}
		return routes, nil
	}
	t.Cleanup(func() { getRoutesFunc = system.ParseProcessRoutes })

	mockSender := mocksender.NewMockSender(t, "docker-network-extension")
	mockSender.SetupAcceptAll()

	mockCollector := mock.NewCollector("testCollector")

	// Test setup:
	// container1 is host network in Kubernetes - linked to PID 100
	// container2 is normal container in Kubernetes (no network config) - linked to container3 and PID 101
	// container3 is a pause container in Kubernetes (owns the network config) - linked to nothing
	// container4 is a normal docker container connected to 2 networks - linked to PIDs 150 (dead) and 200
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
	container1RawDocker := container.Summary{
		ID:    "kube-host-network",
		State: container.ContainerState(workloadmeta.ContainerStatusRunning),
		HostConfig: struct {
			NetworkMode string            `json:",omitempty"`
			Annotations map[string]string `json:",omitempty"`
		}{NetworkMode: "host"},
		NetworkSettings: &container.NetworkSettingsSummary{
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
	container2RawDocker := container.Summary{
		ID:    "kube-app",
		State: container.ContainerState(workloadmeta.ContainerStatusRunning),
		HostConfig: struct {
			NetworkMode string            `json:",omitempty"`
			Annotations map[string]string `json:",omitempty"`
		}{NetworkMode: "container:kube-app-pause"},
		NetworkSettings: &container.NetworkSettingsSummary{
			Networks: map[string]*dockerNetworkTypes.EndpointSettings{},
		},
	}

	// Container3 is only raw as it's excluded (pause container)
	container3RawDocker := container.Summary{
		ID:    "kube-app-pause",
		State: container.ContainerState(workloadmeta.ContainerStatusRunning),
		HostConfig: struct {
			NetworkMode string            `json:",omitempty"`
			Annotations map[string]string `json:",omitempty"`
		}{NetworkMode: "none"},
		NetworkSettings: &container.NetworkSettingsSummary{
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
		// Once the kernel PID counter has wrapped around, the short-lived children of a
		// container get PIDs below the one of its main process. Here 150 belongs to such a
		// child, which already exited, and only the main process (200) can be used to read
		// the container routing table.
		PIDs: []int{150, 200},
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
	container4RawDocker := container.Summary{
		ID:    "docker-app",
		State: container.ContainerState(workloadmeta.ContainerStatusRunning),
		HostConfig: struct {
			NetworkMode string            `json:",omitempty"`
			Annotations map[string]string `json:",omitempty"`
		}{NetworkMode: "ubuntu_default"},
		NetworkSettings: &container.NetworkSettingsSummary{
			Networks: map[string]*dockerNetworkTypes.EndpointSettings{
				"ubuntu_default": {
					IPAddress: netip.MustParseAddr("172.18.0.2"),
				},
				"bridge": {
					IPAddress: netip.MustParseAddr("172.17.0.2"),
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
	dockerNetworkExtension.PostProcess(nooptagger.NewComponent())

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
	networkExt.processContainer(container.Summary{
		ID:      "e2d5394a5321d4a59497f53552a0131b2aafe64faba37f4738e78c531289fc45",
		Names:   []string{"agent"},
		Image:   "datadog/agent",
		ImageID: "sha256:7e813d42985b2e5a0269f868aaf238ffc952a877fba964f55aa1ff35fd0bf5f6",
		Labels: map[string]string{
			"io.kubernetes.pod.namespace": "kubens",
		},
		State:      container.ContainerState(workloadmeta.ContainerStatusRunning),
		SizeRw:     100,
		SizeRootFs: 200,
	})
	networkExt.postRun()
}

// TestFindDockerNetworksPIDSelection covers the PID selection when reading the container routing
// table. Regression test for https://github.com/DataDog/datadog-agent/issues/55676: the first PID
// of the list is not necessarily the container main process, and is often already gone once the
// kernel PID counter has wrapped around.
func TestFindDockerNetworksPIDSelection(t *testing.T) {
	// PIDs shaped like the ones reported in the issue: the main process has been running
	// since before the PID counter wrapped around, so it holds a PID above the ones now
	// handed out to the short-lived children it forks.
	const livePID = 2280542
	const deadPID, otherDeadPID = 1510160, 1511051
	// PID whose routes cannot be read for a reason that is not tied to that PID, e.g. EACCES.
	const deniedPID = 1600000

	rawContainer := container.Summary{
		ID:    "docker-app",
		State: container.ContainerState(workloadmeta.ContainerStatusRunning),
		HostConfig: struct {
			NetworkMode string            `json:",omitempty"`
			Annotations map[string]string `json:",omitempty"`
		}{NetworkMode: "ubuntu_default"},
		NetworkSettings: &container.NetworkSettingsSummary{
			Networks: map[string]*dockerNetworkTypes.EndpointSettings{
				"ubuntu_default": {
					IPAddress: netip.MustParseAddr("172.18.0.2"),
				},
			},
		},
	}

	liveRoutes := []system.NetworkRoute{
		{
			Interface: "eth1",
			Subnet:    0x000012AC,
			Mask:      0x0000FFFF,
			Gateway:   0x00000000,
		},
	}
	expectedMapping := map[string]string{"eth1": "ubuntu_default"}

	tests := []struct {
		name            string
		pids            []int
		expectedMapping map[string]string
		expectedTried   []int
	}{
		{
			name:            "first PID is alive",
			pids:            []int{livePID, deadPID},
			expectedMapping: expectedMapping,
			expectedTried:   []int{livePID},
		},
		{
			name:            "first PIDs are gone, falls back to the live one",
			pids:            []int{deadPID, otherDeadPID, livePID},
			expectedMapping: expectedMapping,
			expectedTried:   []int{deadPID, otherDeadPID, livePID},
		},
		{
			name:            "all PIDs are gone",
			pids:            []int{deadPID, otherDeadPID},
			expectedMapping: nil,
			expectedTried:   []int{deadPID, otherDeadPID},
		},
		{
			name:            "no PID at all",
			pids:            nil,
			expectedMapping: nil,
			expectedTried:   nil,
		},
		{
			name:            "permanent failure stops at the first PID",
			pids:            []int{deniedPID, livePID},
			expectedMapping: nil,
			expectedTried:   []int{deniedPID},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var tried []int
			getRoutesFunc = func(_ string, pid int) ([]system.NetworkRoute, error) {
				tried = append(tried, pid)
				switch pid {
				case livePID:
					return liveRoutes, nil
				case deniedPID:
					return nil, fmt.Errorf("unable to read routes for pid %d: %w", pid, os.ErrPermission)
				default:
					return nil, fmt.Errorf("unable to read routes for pid %d: %w", pid, os.ErrNotExist)
				}
			}
			t.Cleanup(func() { getRoutesFunc = system.ParseProcessRoutes })

			entry := &containerNetworkEntry{containerID: rawContainer.ID, pids: test.pids}
			findDockerNetworks("/proc", entry, rawContainer)

			assert.Equal(t, test.expectedMapping, entry.ifaceNetworkMapping)
			assert.Equal(t, test.expectedTried, tried)
		})
	}
}
