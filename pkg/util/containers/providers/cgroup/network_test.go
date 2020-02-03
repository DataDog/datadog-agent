// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package cgroup

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
)

func TestCollectNetworkStats(t *testing.T) {
	dummyProcDir, err := newTempFolder("test-find-docker-networks")
	assert.Nil(t, err)
	defer dummyProcDir.removeAll() // clean up
	config.Datadog.SetDefault("container_proc_root", dummyProcDir.RootPath)
	defer config.Datadog.SetDefault("container_proc_root", "/proc")

	for _, tc := range []struct {
		pid        int
		name       string
		dev        string
		networks   map[string]string
		stat       metrics.ContainerNetStats
		summedStat *metrics.InterfaceNetStats
	}{
		{
			pid:  1245,
			name: "one-container-interface",
			dev: detab(`
                Inter-|   Receive                                                |  Transmit
                 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
                  eth0:    1345      10    0    0    0     0          0         0        0       0    0    0    0     0       0          0
                    lo:       0       0    0    0    0     0          0         0        0       0    0    0    0     0       0          0
            `),
			networks: map[string]string{
				"eth0": "bridge",
			},
			stat: metrics.ContainerNetStats{
				&metrics.InterfaceNetStats{
					NetworkName: "bridge",
					BytesRcvd:   1345,
					PacketsRcvd: 10,
					BytesSent:   0,
					PacketsSent: 0,
				},
			},
			summedStat: &metrics.InterfaceNetStats{
				BytesRcvd:   1345,
				PacketsRcvd: 10,
				BytesSent:   0,
				PacketsSent: 0,
			},
		},
		// Multiple docker networks
		{
			pid:  5153,
			name: "multiple-networks",
			dev: detab(`
                Inter-|   Receive                                                |  Transmit
                 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
                    lo:       0       0    0    0    0     0          0         0        0       0    0    0    0     0       0          0
                  eth0:     648       8    0    0    0     0          0         0        0       0    0    0    0     0       0          0
                  eth1:    1478      19    0    0    0     0          0         0      182       3    0    0    0     0       0          0`),
			networks: map[string]string{
				"eth0": "bridge",
				"eth1": "test",
			},
			stat: metrics.ContainerNetStats{
				&metrics.InterfaceNetStats{
					NetworkName: "bridge",
					BytesRcvd:   648,
					PacketsRcvd: 8,
					BytesSent:   0,
					PacketsSent: 0,
				},
				&metrics.InterfaceNetStats{
					NetworkName: "test",
					BytesRcvd:   1478,
					PacketsRcvd: 19,
					BytesSent:   182,
					PacketsSent: 3,
				},
			},
			summedStat: &metrics.InterfaceNetStats{
				BytesRcvd:   2126,
				PacketsRcvd: 27,
				BytesSent:   182,
				PacketsSent: 3,
			},
		},
		// Fallback to interface name if network not in map
		{
			pid:  5155,
			name: "multiple-ifaces-missing-network",
			dev: detab(`
                Inter-|   Receive                                                |  Transmit
                 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
                    lo:       0       0    0    0    0     0          0         0        0       0    0    0    0     0       0          0
                  eth0:     648       8    0    0    0     0          0         0        0       0    0    0    0     0       0          0
                  eth1:    1478      19    0    0    0     0          0         0      182       3    0    0    0     0       0          0`),
			networks: map[string]string{
				"eth1": "test",
			},
			stat: metrics.ContainerNetStats{
				&metrics.InterfaceNetStats{
					NetworkName: "eth0",
					BytesRcvd:   648,
					PacketsRcvd: 8,
					BytesSent:   0,
					PacketsSent: 0,
				},
				&metrics.InterfaceNetStats{
					NetworkName: "test",
					BytesRcvd:   1478,
					PacketsRcvd: 19,
					BytesSent:   182,
					PacketsSent: 3,
				},
			},
			summedStat: &metrics.InterfaceNetStats{
				BytesRcvd:   2126,
				PacketsRcvd: 27,
				BytesSent:   182,
				PacketsSent: 3,
			},
		},
		// Dumb error case to make sure we don't panic, fallback to interface name
		{
			pid:  5157,
			name: "nil-network-map",
			dev: detab(`
                Inter-|   Receive                                                |  Transmit
                 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
                  eth0:    1111       2    0    0    0     0          0         0     1024      80    0    0    0     0       0          0
                    lo:       0       0    0    0    0     0          0         0        0       0    0    0    0     0       0          0
            `),
			networks: nil,
			stat: metrics.ContainerNetStats{
				&metrics.InterfaceNetStats{
					NetworkName: "eth0",
					BytesRcvd:   1111,
					PacketsRcvd: 2,
					BytesSent:   1024,
					PacketsSent: 80,
				},
			},
			summedStat: &metrics.InterfaceNetStats{
				BytesRcvd:   1111,
				PacketsRcvd: 2,
				BytesSent:   1024,
				PacketsSent: 80,
			},
		},
	} {
		t.Run("", func(t *testing.T) {
			err = dummyProcDir.add(filepath.Join(strconv.Itoa(tc.pid), "net", "dev"), tc.dev)
			assert.NoError(t, err)

			stat, err := collectNetworkStats(tc.pid, tc.networks)
			assert.NoError(t, err)
			assert.Equal(t, tc.stat, stat)
			assert.Equal(t, tc.summedStat, stat.SumInterfaces())
		})
	}
}

func TestDetectNetworkDestinations(t *testing.T) {
	dummyProcDir, err := newTempFolder("test-find-docker-networks")
	assert.Nil(t, err)
	defer dummyProcDir.removeAll() // clean up
	config.Datadog.SetDefault("container_proc_root", dummyProcDir.RootPath)

	for _, tc := range []struct {
		pid          int
		routes       string
		destinations []containers.NetworkDestination
	}{
		// One interface
		{
			pid: 1245,
			routes: detab(`
                Iface   Destination Gateway     Flags   RefCnt  Use Metric  Mask        MTU Window  IRTT
                eth0    00000000    010011AC    0003    0   0   0   00000000    0   0   0
                eth0    000011AC    00000000    0001    0   0   0   0000FFFF    0   0   0
            `),
			destinations: []containers.NetworkDestination{{
				Interface: "eth0",
				Subnet:    0x000011AC,
				Mask:      0x0000FFFF,
			}},
		},

		// previous int32 overflow bug, now we parse uint32
		{
			pid: 1249,
			routes: detab(`
                Iface   Destination Gateway     Flags   RefCnt  Use Metric  Mask        MTU Window  IRTT
                eth0    00000000    FEFEA8C0    0003    0   0   0   00000000    0   0   0
                eth0    00FEA8C0    00000000    0001    0   0   0   00FFFFFF    0   0   0
			`),
			destinations: []containers.NetworkDestination{{
				Interface: "eth0",
				Subnet:    0x00FEA8C0,
				Mask:      0x00FFFFFF,
			}},
		},
		// Multiple interfaces
		{
			pid: 5153,
			routes: detab(`
                Iface   Destination Gateway     Flags   RefCnt  Use Metric  Mask        MTU Window  IRTT
                eth0    00000000    010011AC    0003    0   0   0   00000000    0   0   0
                eth0    000011AC    00000000    0001    0   0   0   0000FFFF    0   0   0
                eth1    000012AC    00000000    0001    0   0   0   0000FFFF    0   0   0
            `),
			destinations: []containers.NetworkDestination{{
				Interface: "eth0",
				Subnet:    0x000011AC,
				Mask:      0x0000FFFF,
			}, {
				Interface: "eth1",
				Subnet:    0x000012AC,
				Mask:      0x0000FFFF,
			}},
		},
	} {
		t.Run("", func(t *testing.T) {
			// Create temporary files on disk with the routes and stats.
			err = dummyProcDir.add(filepath.Join(strconv.Itoa(tc.pid), "net", "route"), tc.routes)
			assert.NoError(t, err)

			dest, err := detectNetworkDestinations(tc.pid)
			assert.NoError(t, err)
			assert.Equal(t, tc.destinations, dest)
		})
	}
}
