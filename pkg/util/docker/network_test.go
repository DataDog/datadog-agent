// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package docker

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/docker/docker/api/types"
	dockernetwork "github.com/docker/docker/api/types/network"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestFindDockerNetworks(t *testing.T) {
	assert := assert.New(t)

	dummyProcDir, err := newTempFolder("test-find-docker-networks")
	assert.Nil(err)
	defer dummyProcDir.removeAll() // clean up
	config.Datadog.SetDefault("container_proc_root", dummyProcDir.RootPath)

	containerNetworks := make(map[string][]dockerNetwork)
	for nb, tc := range []struct {
		pid         int
		container   types.Container
		routes, dev string
		networks    []dockerNetwork
		stat        ContainerNetStats
		summedStat  *InterfaceNetStats
	}{
		// Host network mode
		{
			pid: 1245,
			container: types.Container{
				ID: "test-find-docker-networks-host",
				HostConfig: struct {
					NetworkMode string `json:",omitempty"`
				}{
					NetworkMode: "host",
				},
				NetworkSettings: &types.SummaryNetworkSettings{},
			},
			routes: detab(`
                Iface   Destination Gateway     Flags   RefCnt  Use Metric  Mask        MTU Window  IRTT
                eth0    00000000    010011AC    0003    0   0   0   00000000    0   0   0
                eth0    000011AC    00000000    0001    0   0   0   0000FFFF    0   0   0
            `),
			dev: detab(`
                Inter-|   Receive                                                |  Transmit
                 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
                  eth0:    1345      10    0    0    0     0          0         0        0       0    0    0    0     0       0          0
                    lo:       0       0    0    0    0     0          0         0        0       0    0    0    0     0       0          0
            `),
			networks: []dockerNetwork{{iface: "eth0", dockerName: "bridge"}},
			stat: ContainerNetStats{
				&InterfaceNetStats{
					NetworkName: "bridge",
					BytesRcvd:   1345,
					PacketsRcvd: 10,
					BytesSent:   0,
					PacketsSent: 0,
				},
			},
			summedStat: &InterfaceNetStats{
				BytesRcvd:   1345,
				PacketsRcvd: 10,
				BytesSent:   0,
				PacketsSent: 0,
			},
		},
		// No network mode, we treat this the same as host for now
		{
			pid: 1245,
			container: types.Container{
				ID: "test-find-docker-networks-nonetwork",
				HostConfig: struct {
					NetworkMode string `json:",omitempty"`
				}{
					NetworkMode: "none",
				},
				NetworkSettings: &types.SummaryNetworkSettings{},
			},
			routes: detab(`
                Iface   Destination Gateway     Flags   RefCnt  Use Metric  Mask        MTU Window  IRTT
                eth0    00000000    010011AC    0003    0   0   0   00000000    0   0   0
                eth0    000011AC    00000000    0001    0   0   0   0000FFFF    0   0   0
            `),
			dev: detab(`
                Inter-|   Receive                                                |  Transmit
                 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
                  eth0:    1345      10    0    0    0     0          0         0        0       0    0    0    0     0       0          0
                    lo:       0       0    0    0    0     0          0         0        0       0    0    0    0     0       0          0
            `),
			networks: []dockerNetwork{{iface: "eth0", dockerName: "bridge"}},
			stat: ContainerNetStats{
				&InterfaceNetStats{
					NetworkName: "bridge",
					BytesRcvd:   1345,
					PacketsRcvd: 10,
					BytesSent:   0,
					PacketsSent: 0,
				},
			},
			summedStat: &InterfaceNetStats{
				BytesRcvd:   1345,
				PacketsRcvd: 10,
				BytesSent:   0,
				PacketsSent: 0,
			},
		},
		// Container network mode
		{
			pid: 1245,
			container: types.Container{
				ID: "test-find-docker-networks-container",
				HostConfig: struct {
					NetworkMode string `json:",omitempty"`
				}{
					NetworkMode: "container:test-find-docker-networks-host",
				},
				NetworkSettings: &types.SummaryNetworkSettings{},
			},
			routes: detab(`
                Iface   Destination Gateway     Flags   RefCnt  Use Metric  Mask        MTU Window  IRTT
                eth0    00000000    010011AC    0003    0   0   0   00000000    0   0   0
                eth0    000011AC    00000000    0001    0   0   0   0000FFFF    0   0   0
            `),
			dev: detab(`
                Inter-|   Receive                                                |  Transmit
                 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
                  eth0:    1345      10    0    0    0     0          0         0        0       0    0    0    0     0       0          0
                    lo:       0       0    0    0    0     0          0         0        0       0    0    0    0     0       0          0
            `),
			networks: []dockerNetwork{{iface: "eth0", dockerName: "bridge"}},
			stat: ContainerNetStats{
				&InterfaceNetStats{
					NetworkName: "bridge",
					BytesRcvd:   1345,
					PacketsRcvd: 10,
					BytesSent:   0,
					PacketsSent: 0,
				},
			},
			summedStat: &InterfaceNetStats{
				BytesRcvd:   1345,
				PacketsRcvd: 10,
				BytesSent:   0,
				PacketsSent: 0,
			},
		},
		{
			pid: 1245,
			container: types.Container{
				ID: "test-find-docker-networks-gateway",
				HostConfig: struct {
					NetworkMode string `json:",omitempty"`
				}{
					NetworkMode: "simple",
				},
				NetworkSettings: &types.SummaryNetworkSettings{
					Networks: map[string]*dockernetwork.EndpointSettings{
						"eth0": {
							Gateway: "172.17.0.1/24",
						},
					},
				},
			},
			routes: detab(`
                Iface   Destination Gateway     Flags   RefCnt  Use Metric  Mask        MTU Window  IRTT
                eth0    00000000    010011AC    0003    0   0   0   00000000    0   0   0
                eth0    000011AC    00000000    0001    0   0   0   0000FFFF    0   0   0
            `),
			dev: detab(`
                Inter-|   Receive                                                |  Transmit
                 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
                  eth0:    1296      16    0    0    0     0          0         0        0       0    0    0    0     0       0          0
                    lo:       0       0    0    0    0     0          0         0        0       0    0    0    0     0       0          0
            `),
			networks: []dockerNetwork{{iface: "eth0", dockerName: "eth0"}},
			stat: ContainerNetStats{
				&InterfaceNetStats{
					NetworkName: "eth0",
					BytesRcvd:   1296,
					PacketsRcvd: 16,
					BytesSent:   0,
					PacketsSent: 0,
				},
			},
			summedStat: &InterfaceNetStats{
				BytesRcvd:   1296,
				PacketsRcvd: 16,
				BytesSent:   0,
				PacketsSent: 0,
			},
		},
		// Multiple docker networks
		{
			pid: 5153,
			container: types.Container{
				ID: "test-find-docker-networks-multiple",
				NetworkSettings: &types.SummaryNetworkSettings{
					Networks: map[string]*dockernetwork.EndpointSettings{
						"bridge": {
							Gateway: "172.17.0.1",
						},
						"test": {
							Gateway: "172.18.0.1",
						},
					},
				},
			},
			routes: detab(`
				Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
				eth0	00000000	010011AC	0003	0	0	0	00000000	0	0	0
				eth0	000011AC	00000000	0001	0	0	0	0000FFFF	0	0	0
				eth1	000012AC	00000000	0001	0	0	0	0000FFFF	0	0	0
            `),
			dev: detab(`
				Inter-|   Receive                                                |  Transmit
				 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
				    lo:       0       0    0    0    0     0          0         0        0       0    0    0    0     0       0          0
				  eth0:     648       8    0    0    0     0          0         0        0       0    0    0    0     0       0          0
				  eth1:    1478      19    0    0    0     0          0         0      182       3    0    0    0     0       0          0`),
			networks: []dockerNetwork{
				{iface: "eth0", dockerName: "bridge"},
				{iface: "eth1", dockerName: "test"},
			},
			stat: ContainerNetStats{
				&InterfaceNetStats{
					NetworkName: "bridge",
					BytesRcvd:   648,
					PacketsRcvd: 8,
					BytesSent:   0,
					PacketsSent: 0,
				},
				&InterfaceNetStats{
					NetworkName: "test",
					BytesRcvd:   1478,
					PacketsRcvd: 19,
					BytesSent:   182,
					PacketsSent: 3,
				},
			},
			summedStat: &InterfaceNetStats{
				BytesRcvd:   2126,
				PacketsRcvd: 27,
				BytesSent:   182,
				PacketsSent: 3,
			},
		},
		// Dumb error case to make sure we don't panic
		{
			pid: 5157,
			container: types.Container{
				ID: "test-find-docker-networks-errcase",
				HostConfig: struct {
					NetworkMode string `json:",omitempty"`
				}{
					NetworkMode: "isolated_nw",
				},
				NetworkSettings: &types.SummaryNetworkSettings{
					Networks: map[string]*dockernetwork.EndpointSettings{
						"isolated_nw": {
							Gateway: "172.18.0.1",
						},
						"eth0": {
							Gateway: "172.0.0.4/24",
						},
					},
				},
			},
			routes:   detab(``),
			networks: nil,
			dev: detab(`
                Inter-|   Receive                                                |  Transmit
                 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
                  eth0:    1111       2    0    0    0     0          0         0     1024      80    0    0    0     0       0          0
                    lo:       0       0    0    0    0     0          0         0        0       0    0    0    0     0       0          0
            `),
			stat:       ContainerNetStats{},
			summedStat: &InterfaceNetStats{},
		},
	} {
		t.Logf("test case %d", nb)
		// Create temporary files on disk with the routes and stats.
		err = dummyProcDir.add(filepath.Join(strconv.Itoa(int(tc.pid)), "net", "route"), tc.routes)
		assert.NoError(err)
		err = dummyProcDir.add(filepath.Join(strconv.Itoa(int(tc.pid)), "net", "dev"), tc.dev)
		assert.NoError(err)

		// Use the routes file and settings to get our networks.
		networks := findDockerNetworks(tc.container.ID, tc.pid, tc.container)
		containerNetworks[tc.container.ID] = networks
		// Resolve any container-dependent networks before checking values.
		// This assumes that the test case can only depend on containers in earlier cases.
		resolveDockerNetworks(containerNetworks)
		networks = containerNetworks[tc.container.ID]
		assert.Equal(tc.networks, networks)

		// And collect the stats on these networks.
		stat, err := collectNetworkStats(tc.container.ID, tc.pid, networks)
		assert.NoError(err)
		assert.Equal(tc.stat, stat)
		assert.Equal(tc.summedStat, stat.SumInterfaces())
	}
}
