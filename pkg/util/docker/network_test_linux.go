// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker && linux
// +build docker,linux

package docker

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	dockernetwork "github.com/docker/docker/api/types/network"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/testutil"
)

func TestFindDockerNetworks(t *testing.T) {
	dummyProcDir, err := testutil.NewTempFolder("test-find-docker-networks")
	assert.Nil(t, err)
	defer dummyProcDir.RemoveAll() //nolint:errcheck // clean up
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
			routes: testutil.Detab(`
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
			routes: testutil.Detab(`
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
			routes: testutil.Detab(`
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
			routes: testutil.Detab(`
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
			routes: testutil.Detab(`
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
			routes: testutil.Detab(`
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
			err = dummyProcDir.Add(filepath.Join(strconv.Itoa(tc.pid), "net", "route"), tc.routes)
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
