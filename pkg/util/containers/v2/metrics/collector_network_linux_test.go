// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package metrics

import (
	"io/ioutil"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/testutil"
)

func TestCollectNetworkStats(t *testing.T) {
	dummyProcDir, err := testutil.NewTempFolder("test-find-docker-networks")
	assert.Nil(t, err)
	defer dummyProcDir.RemoveAll() // clean up

	for _, tc := range []struct {
		pid        int
		name       string
		dev        string
		networks   map[string]string
		stat       ContainerNetworkStats
		summedStat *InterfaceNetStats
	}{
		{
			pid:  1245,
			name: "one-container-interface",
			dev: testutil.Detab(`
                Inter-|   Receive                                                |  Transmit
                 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
                  eth0:    1345      10    0    0    0     0          0         0        0       0    0    0    0     0       0          0
                    lo:       0       0    0    0    0     0          0         0        0       0    0    0    0     0       0          0
            `),
			networks: map[string]string{
				"eth0": "bridge",
			},
			stat: ContainerNetworkStats{
				Interfaces: map[string]InterfaceNetStats{
					"bridge": {
						BytesRcvd:   util.Float64Ptr(1345),
						PacketsRcvd: util.Float64Ptr(10),
						BytesSent:   util.Float64Ptr(0),
						PacketsSent: util.Float64Ptr(0),
					},
				},
				BytesRcvd:   util.Float64Ptr(1345),
				PacketsRcvd: util.Float64Ptr(10),
				BytesSent:   util.Float64Ptr(0),
				PacketsSent: util.Float64Ptr(0),
			},
		},
		// Multiple docker networks
		{
			pid:  5153,
			name: "multiple-networks",
			dev: testutil.Detab(`
                Inter-|   Receive                                                |  Transmit
                 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
                    lo:       0       0    0    0    0     0          0         0        0       0    0    0    0     0       0          0
                  eth0:     648       8    0    0    0     0          0         0        0       0    0    0    0     0       0          0
                  eth1:    1478      19    0    0    0     0          0         0      182       3    0    0    0     0       0          0`),
			networks: map[string]string{
				"eth0": "bridge",
				"eth1": "test",
			},
			stat: ContainerNetworkStats{
				Interfaces: map[string]InterfaceNetStats{
					"bridge": {
						BytesRcvd:   util.Float64Ptr(648),
						PacketsRcvd: util.Float64Ptr(8),
						BytesSent:   util.Float64Ptr(0),
						PacketsSent: util.Float64Ptr(0),
					},
					"test": {
						BytesRcvd:   util.Float64Ptr(1478),
						PacketsRcvd: util.Float64Ptr(19),
						BytesSent:   util.Float64Ptr(182),
						PacketsSent: util.Float64Ptr(3),
					},
				},
				BytesRcvd:   util.Float64Ptr(2126),
				PacketsRcvd: util.Float64Ptr(27),
				BytesSent:   util.Float64Ptr(182),
				PacketsSent: util.Float64Ptr(3),
			},
		},
		// Fallback to interface name if network not in map
		{
			pid:  5155,
			name: "multiple-ifaces-missing-network",
			dev: testutil.Detab(`
                Inter-|   Receive                                                |  Transmit
                 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
                    lo:       0       0    0    0    0     0          0         0        0       0    0    0    0     0       0          0
                  eth0:     648       8    0    0    0     0          0         0        0       0    0    0    0     0       0          0
                  eth1:    1478      19    0    0    0     0          0         0      182       3    0    0    0     0       0          0`),
			networks: map[string]string{
				"eth1": "test",
			},
			stat: ContainerNetworkStats{
				Interfaces: map[string]InterfaceNetStats{
					"eth0": {
						BytesRcvd:   util.Float64Ptr(648),
						PacketsRcvd: util.Float64Ptr(8),
						BytesSent:   util.Float64Ptr(0),
						PacketsSent: util.Float64Ptr(0),
					},
					"test": {
						BytesRcvd:   util.Float64Ptr(1478),
						PacketsRcvd: util.Float64Ptr(19),
						BytesSent:   util.Float64Ptr(182),
						PacketsSent: util.Float64Ptr(3),
					},
				},
				BytesRcvd:   util.Float64Ptr(2126),
				PacketsRcvd: util.Float64Ptr(27),
				BytesSent:   util.Float64Ptr(182),
				PacketsSent: util.Float64Ptr(3),
			},
		},
		// Dumb error case to make sure we don't panic, fallback to interface name
		{
			pid:  5157,
			name: "nil-network-map",
			dev: testutil.Detab(`
                Inter-|   Receive                                                |  Transmit
                 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
                  eth0:    1111       2    0    0    0     0          0         0     1024      80    0    0    0     0       0          0
                    lo:       0       0    0    0    0     0          0         0        0       0    0    0    0     0       0          0
            `),
			networks: nil,
			stat: ContainerNetworkStats{
				Interfaces: map[string]InterfaceNetStats{
					"eth0": {
						BytesRcvd:   util.Float64Ptr(1111),
						PacketsRcvd: util.Float64Ptr(2),
						BytesSent:   util.Float64Ptr(1024),
						PacketsSent: util.Float64Ptr(80),
					},
				},
				BytesRcvd:   util.Float64Ptr(1111),
				PacketsRcvd: util.Float64Ptr(2),
				BytesSent:   util.Float64Ptr(1024),
				PacketsSent: util.Float64Ptr(80),
			},
		},
	} {
		t.Run("", func(t *testing.T) {
			err = dummyProcDir.Add(filepath.Join(strconv.Itoa(tc.pid), "net", "dev"), tc.dev)
			assert.NoError(t, err)

			stat, err := collectNetworkStats(dummyProcDir.RootPath, tc.pid, tc.networks)
			assert.NoError(t, err)
			assert.Equal(t, &tc.stat, stat)
		})
	}
}

func TestDetectNetworkDestinations(t *testing.T) {
	dummyProcDir, err := testutil.NewTempFolder("test-find-docker-networks")
	assert.Nil(t, err)
	defer dummyProcDir.RemoveAll() // clean up

	for _, tc := range []struct {
		pid          int
		routes       string
		destinations []containers.NetworkDestination
	}{
		// One interface
		{
			pid: 1245,
			routes: testutil.Detab(`
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
			routes: testutil.Detab(`
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
			routes: testutil.Detab(`
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
			err = dummyProcDir.Add(filepath.Join(strconv.Itoa(tc.pid), "net", "route"), tc.routes)
			assert.NoError(t, err)

			dest, err := detectNetworkDestinations(dummyProcDir.RootPath, tc.pid)
			assert.NoError(t, err)
			assert.Equal(t, tc.destinations, dest)
		})
	}
}

func TestDefaultGateway(t *testing.T) {
	testCases := []struct {
		netRouteContent []byte
		expectedIP      string
	}{
		{
			[]byte(`Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
ens33	00000000	0280A8C0	0003	0	0	100	00000000	0	0	0
`),
			"192.168.128.2",
		},
		{
			[]byte(`Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
ens33	00000000	FE01A8C0	0003	0	0	100	00000000	0	0	0
`),
			"192.168.1.254",
		},
		{
			[]byte(`Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
ens33	00000000	FEFEA8C0	0003	0	0	100	00000000	0	0	0
`),
			"192.168.254.254",
		},
	}
	for _, testCase := range testCases {
		t.Run("", func(t *testing.T) {
			testProc, err := testutil.NewTempFolder("test-default-gateway")
			require.NoError(t, err)
			defer testProc.RemoveAll()
			err = os.MkdirAll(path.Join(testProc.RootPath, "net"), os.ModePerm)
			require.NoError(t, err)

			err = ioutil.WriteFile(path.Join(testProc.RootPath, "net", "route"), testCase.netRouteContent, os.ModePerm)
			require.NoError(t, err)
			ip, err := defaultGateway(testProc.RootPath)
			require.NoError(t, err)
			assert.Equal(t, testCase.expectedIP, ip.String())

			testProc.RemoveAll()
			ip, err = defaultGateway(testProc.RootPath)
			require.NoError(t, err)
			require.Nil(t, ip)
		})
	}
}

func TestDefaulHostIPs(t *testing.T) {
	dummyProcDir, err := testutil.NewTempFolder("test-default-host-ips")
	require.Nil(t, err)
	defer dummyProcDir.RemoveAll()

	t.Run("routing table contains a gateway entry", func(t *testing.T) {
		routes := `
		    Iface    Destination Gateway  Flags RefCnt Use Metric Mask     MTU Window IRTT
		    default  00000000    010011AC 0003  0      0   0      00000000 0   0      0
		    default  000011AC    00000000 0001  0      0   0      0000FFFF 0   0      0
		    eth1     000012AC    00000000 0001  0      0   0      0000FFFF 0   0      0 `

		// Pick an existing device and replace the "default" placeholder by its name
		interfaces, err := net.Interfaces()
		require.NoError(t, err)
		require.NotEmpty(t, interfaces)
		netInterface := interfaces[0]
		routes = strings.ReplaceAll(routes, "default", netInterface.Name)

		// Populate routing table file
		err = dummyProcDir.Add(filepath.Join("net", "route"), routes)
		require.NoError(t, err)

		// Retrieve IPs bound to the "default" network interface
		var expectedIPs []string
		netAddrs, err := netInterface.Addrs()
		require.NoError(t, err)
		require.NotEmpty(t, netAddrs)
		for _, address := range netAddrs {
			ip := strings.Split(address.String(), "/")[0]
			require.NotNil(t, net.ParseIP(ip))
			expectedIPs = append(expectedIPs, ip)
		}

		// Verify they match the IPs returned by DefaultHostIPs()
		defaultIPs, err := defaultHostIPs(dummyProcDir.RootPath)
		assert.Nil(t, err)
		assert.Equal(t, expectedIPs, defaultIPs)
	})

	t.Run("routing table missing a gateway entry", func(t *testing.T) {
		routes := `
	        Iface    Destination Gateway  Flags RefCnt Use Metric Mask     MTU Window IRTT
	        eth0     000011AC    00000000 0001  0      0   0      0000FFFF 0   0      0
	        eth1     000012AC    00000000 0001  0      0   0      0000FFFF 0   0      0 `

		err = dummyProcDir.Add(filepath.Join("net", "route"), routes)
		require.NoError(t, err)
		ips, err := defaultHostIPs(dummyProcDir.RootPath)
		assert.Nil(t, ips)
		assert.NotNil(t, err)
	})
}
