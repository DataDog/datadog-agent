// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package docker

import (
	"path/filepath"
	"strconv"
	"strings"
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
	for _, tc := range []struct {
		pid         int
		settings    *types.SummaryNetworkSettings
		routes, dev string
		networks    []dockerNetwork
		stat        *NetworkStat
	}{
		{
			pid: 1245,
			settings: &types.SummaryNetworkSettings{
				Networks: map[string]*dockernetwork.EndpointSettings{
					"eth0": &dockernetwork.EndpointSettings{
						Gateway: "172.0.0.1/24",
					},
				},
			},
			routes: detab(`
				Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
				eth0	00000000	010011AC	0003	0	0	0	00000000	0	0	0

				eth0	000011AC	00000000	0001	0	0	0	0000FFFF	0	0	0
			`),
			dev: detab(`
				Inter-|   Receive                                                |  Transmit
				 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
				  eth0:    1296      16    0    0    0     0          0         0        0       0    0    0    0     0       0          0
				    lo:       0       0    0    0    0     0          0         0        0       0    0    0    0     0       0          0
			`),
			networks: []dockerNetwork{dockerNetwork{iface: "eth0", dockerName: "eth0"}},
			stat: &NetworkStat{
				BytesRcvd:   1296,
				PacketsRcvd: 16,
				BytesSent:   0,
				PacketsSent: 0,
			},
		},
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
				Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
				eth0	00000000	010012AC	0003	0	0	0	00000000	0	0	0

				eth0	000012AC	00000000	0001	0	0	0	0000FFFF	0	0	0
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
			stat: &NetworkStat{
				BytesRcvd:   1111,
				PacketsRcvd: 2,
				BytesSent:   1024,
				PacketsSent: 80,
			},
		},
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
			stat: &NetworkStat{},
		},
	} {
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
	}
}

// detab removes whitespace from the front of a string on every line
func detab(str string) string {
	detabbed := make([]string, 0)
	for _, l := range strings.Split(str, "\n") {
		s := strings.TrimSpace(l)
		if len(s) > 0 {
			detabbed = append(detabbed, s)
		}
	}
	return strings.Join(detabbed, "\n")
}

// Sanity-check that all containers works with different settings.
func TestAllContainers(t *testing.T) {
	InitDockerUtil(&Config{CollectNetwork: true})
	AllContainers(&ContainerListConfig{IncludeExited: false, FlagExcluded: true})
	AllContainers(&ContainerListConfig{IncludeExited: true, FlagExcluded: true})
	InitDockerUtil(&Config{CollectNetwork: false})
	AllContainers(&ContainerListConfig{IncludeExited: false, FlagExcluded: true})
	AllContainers(&ContainerListConfig{IncludeExited: true, FlagExcluded: true})
}

func TestParseContainerHealth(t *testing.T) {
	assert := assert.New(t)
	for i, tc := range []struct {
		input    string
		expected string
	}{
		{
			input:    "",
			expected: "",
		},
		{
			input:    "Up 2 minutes",
			expected: "",
		},
		{
			input:    "Up about 1 hour (health: starting)",
			expected: "starting",
		},
		{
			input:    "Up 1 minute (health: unhealthy)",
			expected: "unhealthy",
		},
	} {
		assert.Equal(tc.expected, parseContainerHealth(tc.input), "test %d failed", i)
	}
}

func TestExtractImageName(t *testing.T) {
	imageName := "datadog/docker-dd-agent:latest"
	imageSha := "sha256:bdc7dc8ba08c2ac8c8e03550d8ebf3297a669a3f03e36c377b9515f08c1b4ef4"
	imageWithShaTag := "datadog/docker-dd-agent@sha256:9aab42bf6a2a068b797fe7d91a5d8d915b10dbbc3d6f2b10492848debfba6044"

	assert := assert.New(t)
	globalDockerUtil = &dockerUtil{
		cfg:            &Config{CollectNetwork: false},
		cli:            nil,
		imageNameBySha: make(map[string]string),
	}
	globalDockerUtil.imageNameBySha[imageWithShaTag] = imageName
	globalDockerUtil.imageNameBySha[imageSha] = imageName
	for i, tc := range []struct {
		input    string
		expected string
	}{
		{
			input:    "",
			expected: "",
		}, {
			input:    imageName,
			expected: imageName,
		}, {
			input:    imageWithShaTag,
			expected: imageName,
		}, {
			input:    imageSha,
			expected: imageName,
		},
	} {
		assert.Equal(tc.expected, globalDockerUtil.extractImageName(tc.input), "test %s failed", i)
	}
}
