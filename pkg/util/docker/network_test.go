// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package docker

import (
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/docker/docker/api/types"
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
