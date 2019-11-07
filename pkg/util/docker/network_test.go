// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package docker

import (
	"io/ioutil"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	dockernetwork "github.com/docker/docker/api/types/network"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
)

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
			testProc, err := newTempFolder("test-default-gateway")
			require.NoError(t, err)
			defer testProc.removeAll()
			err = os.MkdirAll(path.Join(testProc.RootPath, "net"), os.ModePerm)
			require.NoError(t, err)

			err = ioutil.WriteFile(path.Join(testProc.RootPath, "net", "route"), testCase.netRouteContent, os.ModePerm)
			require.NoError(t, err)
			config.Datadog.SetDefault("proc_root", testProc.RootPath)
			ip, err := DefaultGateway()
			require.NoError(t, err)
			assert.Equal(t, testCase.expectedIP, ip.String())

			testProc.removeAll()
			ip, err = DefaultGateway()
			require.NoError(t, err)
			require.Nil(t, ip)
		})
	}
}

func TestDefaulHostIPs(t *testing.T) {
	dummyProcDir, err := newTempFolder("test-default-host-ips")
	require.Nil(t, err)
	defer dummyProcDir.removeAll()
	config.Datadog.SetDefault("proc_root", dummyProcDir.RootPath)

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
		err = dummyProcDir.add(filepath.Join("net", "route"), routes)
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
		defaultIPs, err := DefaultHostIPs()
		assert.Nil(t, err)
		assert.Equal(t, expectedIPs, defaultIPs)
	})

	t.Run("routing table missing a gateway entry", func(t *testing.T) {
		routes := `
	        Iface    Destination Gateway  Flags RefCnt Use Metric Mask     MTU Window IRTT
	        eth0     000011AC    00000000 0001  0      0   0      0000FFFF 0   0      0
	        eth1     000012AC    00000000 0001  0      0   0      0000FFFF 0   0      0 `

		err = dummyProcDir.add(filepath.Join("net", "route"), routes)
		require.NoError(t, err)
		ips, err := DefaultHostIPs()
		assert.Nil(t, ips)
		assert.NotNil(t, err)
	})
}

func TestFindDockerNetworks(t *testing.T) {
	dummyProcDir, err := newTempFolder("test-find-docker-networks")
	assert.Nil(t, err)
	defer dummyProcDir.removeAll() // clean up
	config.Datadog.SetDefault("container_proc_root", dummyProcDir.RootPath)

	containerNetworks := make(map[string][]dockerNetwork)
	for _, tc := range []struct {
		pid       int
		container types.Container
		routes    string
		networks  []dockerNetwork
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
			networks: []dockerNetwork{{iface: "eth0", dockerName: "bridge"}},
		},
		// No network mode, we treat this the same as host for now
		{
			pid: 1246,
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
			networks: []dockerNetwork{{iface: "eth0", dockerName: "bridge"}},
		},
		// Container network mode
		{
			pid: 1247,
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
			networks: []dockerNetwork{{iface: "eth0", dockerName: "bridge"}},
		},
		{
			pid: 1248,
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
							IPAddress: "172.17.0.1",
						},
					},
				},
			},
			routes: detab(`
                Iface   Destination Gateway     Flags   RefCnt  Use Metric  Mask        MTU Window  IRTT
                eth0    00000000    010011AC    0003    0   0   0   00000000    0   0   0
                eth0    000011AC    00000000    0001    0   0   0   0000FFFF    0   0   0
            `),
			networks: []dockerNetwork{{iface: "eth0", dockerName: "eth0"}},
		},
		// previous int32 overflow bug, now we parse uint32
		{
			pid: 1249,
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
							IPAddress: "192.168.254.254",
						},
					},
				},
			},
			routes: detab(`
		                Iface   Destination Gateway     Flags   RefCnt  Use Metric  Mask        MTU Window  IRTT
		                eth0    00000000    FEFEA8C0    0003    0   0   0   00000000    0   0   0
		                eth0    00FEA8C0    00000000    0001    0   0   0   00FFFFFF    0   0   0
		            `),
			networks: []dockerNetwork{{iface: "eth0", dockerName: "eth0"}},
		},
		// Multiple docker networks
		{
			pid: 5153,
			container: types.Container{
				ID: "test-find-docker-networks-multiple",
				NetworkSettings: &types.SummaryNetworkSettings{
					Networks: map[string]*dockernetwork.EndpointSettings{
						"bridge": {
							IPAddress: "172.17.0.1",
						},
						"test": {
							IPAddress: "172.18.0.1",
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
			networks: []dockerNetwork{
				{iface: "eth0", dockerName: "bridge"},
				{iface: "eth1", dockerName: "test"},
			},
		},
	} {
		t.Run("", func(t *testing.T) {
			// Create temporary files on disk with the routes and stats.
			err = dummyProcDir.add(filepath.Join(strconv.Itoa(int(tc.pid)), "net", "route"), tc.routes)
			assert.NoError(t, err)

			// Use the routes file and settings to get our networks.
			networks := findDockerNetworks(tc.container.ID, tc.pid, tc.container)
			containerNetworks[tc.container.ID] = networks
			// Resolve any container-dependent networks before checking values.
			// This assumes that the test case can only depend on containers in earlier cases.
			resolveDockerNetworks(containerNetworks)
			networks = containerNetworks[tc.container.ID]
			assert.Equal(t, tc.networks, networks)
		})
	}
}

func TestParseContainerNetworkMode(t *testing.T) {
	tests := []struct {
		name       string
		hostConfig *container.HostConfig
		want       string
		wantErr    bool
	}{
		{
			name: "default",
			hostConfig: &container.HostConfig{
				NetworkMode: "default",
			},
			want:    "default",
			wantErr: false,
		},
		{
			name: "host",
			hostConfig: &container.HostConfig{
				NetworkMode: "host",
			},
			want:    "host",
			wantErr: false,
		},
		{
			name: "bridge",
			hostConfig: &container.HostConfig{
				NetworkMode: "bridge",
			},
			want:    "bridge",
			wantErr: false,
		},
		{
			name: "none",
			hostConfig: &container.HostConfig{
				NetworkMode: "none",
			},
			want:    "none",
			wantErr: false,
		},
		{
			name: "attached to container",
			hostConfig: &container.HostConfig{
				NetworkMode: "container:0a8f83f35f7d0161f29b819d9b533b57acade8d99609bba63664dd3326e4d301",
			},
			want:    "container:0a8f83f35f7d0161f29b819d9b533b57acade8d99609bba63664dd3326e4d301",
			wantErr: false,
		},
		{
			name: "unknown",
			hostConfig: &container.HostConfig{
				NetworkMode: "unknown network",
			},
			want:    "unknown",
			wantErr: true,
		},
		{
			name:       "nil hostConfig",
			hostConfig: nil,
			want:       "",
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseContainerNetworkMode(tt.hostConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseContainerNetworkMode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseContainerNetworkMode() = %v, want %v", got, tt.want)
			}
		})
	}
}
