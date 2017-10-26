// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

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

	containerID := "test-find-docker-networks"
	for nb, tc := range []struct {
		pid         int
		settings    *types.SummaryNetworkSettings
		routes, dev string
		networks    []dockerNetwork
		stat        ContainerNetStats
		summedStat  *InterfaceNetStats
	}{
		{
			pid: 1245,
			settings: &types.SummaryNetworkSettings{
				Networks: map[string]*dockernetwork.EndpointSettings{
					"eth0": &dockernetwork.EndpointSettings{
						Gateway: "172.17.0.1/24",
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
			networks: []dockerNetwork{dockerNetwork{iface: "eth0", dockerName: "eth0"}},
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
			settings: &types.SummaryNetworkSettings{
				Networks: map[string]*dockernetwork.EndpointSettings{
					"bridge": &dockernetwork.EndpointSettings{
						Gateway: "172.17.0.1",
					},
					"test": &dockernetwork.EndpointSettings{
						Gateway: "172.18.0.1",
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
				dockerNetwork{iface: "eth0", dockerName: "bridge"},
				dockerNetwork{iface: "eth1", dockerName: "test"},
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
		/* TODO: where is this example from?
				{
					pid: 5152,
					settings: &types.SummaryNetworkSettings{
						Networks: map[string]*dockernetwork.EndpointSettings{
							"isolated_nw": &dockernetwork.EndpointSettings{
								Gateway: "172.18.0.1",
							},
							"eth0": &dockernetwork.EndpointSettings{
								Gateway: "172.0.0.4/24",
							},
						},
					},
					routes: detab(`
		                Iface   Destination Gateway     Flags   RefCnt  Use Metric  Mask        MTU Window  IRTT
		                eth0    00000000    010012AC    0003    0   0   0   00000000    0   0   0

		                eth0    000012AC    00000000    0001    0   0   0   0000FFFF    0   0   0
		            `),
					dev: detab(`
		                Inter-|   Receive                                                |  Transmit
		                 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
		                  eth0:    1111       2    0    0    0     0          0         0     1024      80    0    0    0     0       0          0
		                    lo:       0       0    0    0    0     0          0         0        0       0    0    0    0     0       0          0
		            `),
					networks: []dockerNetwork{
						dockerNetwork{iface: "eth0", dockerName: "eth0"},
						dockerNetwork{iface: "eth0", dockerName: "isolated_nw"},
					},
					stat: &ContainerNetStats{
						Stats: []*InterfaceNetStats{
							&InterfaceNetStats{
								NetworkName: "isolated_nw",
								BytesRcvd:   1111,
								PacketsRcvd: 2,
								BytesSent:   1024,
								PacketsSent: 80,
							},
						},
					},
					summedStat: &InterfaceNetStats{
						BytesRcvd:   1111,
						PacketsRcvd: 2,
						BytesSent:   1024,
						PacketsSent: 80,
					},
				},
		*/
		// Dumb error case to make sure we don't panic
		{
			pid: 5157,
			settings: &types.SummaryNetworkSettings{
				Networks: map[string]*dockernetwork.EndpointSettings{
					"isolated_nw": &dockernetwork.EndpointSettings{
						Gateway: "172.18.0.1",
					},
					"eth0": &dockernetwork.EndpointSettings{
						Gateway: "172.0.0.4/24",
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
		networks := findDockerNetworks(containerID, tc.pid, tc.settings)
		assert.Equal(tc.networks, networks)

		// And collect the stats on these networks.
		stat, err := collectNetworkStats(containerID, tc.pid, networks)
		assert.NoError(err)
		assert.Equal(tc.stat, stat)
		assert.Equal(tc.summedStat, stat.SumInterfaces())

	}
}
