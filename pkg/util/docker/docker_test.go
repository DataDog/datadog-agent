// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package docker

import (
	"net"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

func TestContainerIDToEntityName(t *testing.T) {
	assert.Equal(t, "", ContainerIDToEntityName(""))
	assert.Equal(t, "docker://ada5d83e6c2d3dfaaf7dd9ff83e735915da1174dc56880c06a6c99a9a58d5c73", ContainerIDToEntityName("ada5d83e6c2d3dfaaf7dd9ff83e735915da1174dc56880c06a6c99a9a58d5c73"))
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
		{
			input:    "Up 1 minute (unhealthy)",
			expected: "unhealthy",
		},
	} {
		assert.Equal(tc.expected, parseContainerHealth(tc.input), "test %d failed", i)
	}
}

func TestResolveImageName(t *testing.T) {
	imageName := "datadog/docker-dd-agent:latest"
	imageSha := "sha256:bdc7dc8ba08c2ac8c8e03550d8ebf3297a669a3f03e36c377b9515f08c1b4ef4"
	imageWithShaTag := "datadog/docker-dd-agent@sha256:9aab42bf6a2a068b797fe7d91a5d8d915b10dbbc3d6f2b10492848debfba6044"

	assert := assert.New(t)
	globalDockerUtil = &DockerUtil{
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
		name, err := globalDockerUtil.ResolveImageName(tc.input)
		assert.Equal(tc.expected, name, "test %s failed", i)
		assert.Nil(err, "test %s failed", i)

	}
}

func TestParseECSContainerNetworkAddresses(t *testing.T) {

	for i, tc := range []struct {
		name         string
		containerID  string
		ports        []types.Port
		netSettings  *types.SummaryNetworkSettings
		cacheContent types.ContainerJSON
		expected     []containers.NetworkAddress
	}{
		{
			name:        "empty input",
			containerID: "foo",
			ports:       []types.Port{},
			netSettings: nil,
			cacheContent: types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{ID: "foo", Image: "org/test"},
				Mounts:            make([]types.MountPoint, 0),
				Config:            &container.Config{},
				NetworkSettings:   &types.NetworkSettings{},
			},
			expected: []containers.NetworkAddress{},
		},
		{
			name:        "exposed port",
			containerID: "foo",
			ports: []types.Port{
				{
					IP:          "0.0.0.0",
					PrivatePort: 80,
					PublicPort:  8080,
					Type:        "tcp",
				},
			},
			netSettings: &types.SummaryNetworkSettings{
				Networks: map[string]*network.EndpointSettings{
					"bridge": {
						IPAMConfig:          nil,
						Links:               nil,
						Aliases:             nil,
						NetworkID:           "NetworkID",
						EndpointID:          "EndpointID",
						Gateway:             "172.17.0.1",
						IPAddress:           "172.17.0.2",
						IPPrefixLen:         16,
						IPv6Gateway:         "",
						GlobalIPv6Address:   "",
						GlobalIPv6PrefixLen: 0,
						MacAddress:          "MacAddress",
					},
				},
			},
			cacheContent: types.ContainerJSON{},
			expected: []containers.NetworkAddress{
				{
					IP:       net.ParseIP("0.0.0.0"),
					Port:     8080,
					Protocol: "tcp",
				},
				{
					IP:       net.ParseIP("172.17.0.2"),
					Port:     80,
					Protocol: "tcp",
				},
			},
		},
		{
			name:        "not exposed port",
			containerID: "foo",
			ports: []types.Port{
				{
					PrivatePort: 80,
					Type:        "tcp",
				},
			},
			netSettings: &types.SummaryNetworkSettings{
				Networks: map[string]*network.EndpointSettings{
					"bridge": {
						IPAMConfig:          nil,
						Links:               nil,
						Aliases:             nil,
						NetworkID:           "NetworkID",
						EndpointID:          "EndpointID",
						Gateway:             "172.17.0.1",
						IPAddress:           "172.17.0.2",
						IPPrefixLen:         16,
						IPv6Gateway:         "",
						GlobalIPv6Address:   "",
						GlobalIPv6PrefixLen: 0,
						MacAddress:          "MacAddress",
					},
				},
			},
			cacheContent: types.ContainerJSON{},
			expected: []containers.NetworkAddress{
				{
					IP:       net.ParseIP("172.17.0.2"),
					Port:     80,
					Protocol: "tcp",
				},
			},
		},
		{
			name:        "empty address info",
			containerID: "foo",
			ports: []types.Port{
				{
					PrivatePort: 80,
					Type:        "tcp",
				},
			},
			netSettings: &types.SummaryNetworkSettings{
				Networks: map[string]*network.EndpointSettings{
					"emptyNetwork": {},
				},
			},
			cacheContent: types.ContainerJSON{},
			expected:     []containers.NetworkAddress{},
		},
		{
			name:        "host network mode",
			containerID: "foo",
			// Published ports are discarded when using host network mode
			ports: []types.Port{},
			netSettings: &types.SummaryNetworkSettings{
				Networks: map[string]*network.EndpointSettings{
					"host": {
						IPAMConfig:          nil,
						Links:               nil,
						Aliases:             nil,
						NetworkID:           "NetworkID",
						EndpointID:          "EndpointID",
						Gateway:             "",
						IPAddress:           "",
						IPPrefixLen:         0,
						IPv6Gateway:         "",
						GlobalIPv6Address:   "",
						GlobalIPv6PrefixLen: 0,
						MacAddress:          "",
					},
				},
			},
			cacheContent: types.ContainerJSON{},
			expected:     []containers.NetworkAddress{},
		},
		{
			name:        "multiple networks",
			containerID: "foo",
			ports: []types.Port{
				{
					PrivatePort: 80,
					Type:        "tcp",
				},
			},
			netSettings: &types.SummaryNetworkSettings{
				Networks: map[string]*network.EndpointSettings{
					"bridge": {
						IPAMConfig:          nil,
						Links:               nil,
						Aliases:             nil,
						NetworkID:           "NetworkID",
						EndpointID:          "EndpointID",
						Gateway:             "172.17.0.1",
						IPAddress:           "172.17.0.2",
						IPPrefixLen:         16,
						IPv6Gateway:         "",
						GlobalIPv6Address:   "",
						GlobalIPv6PrefixLen: 0,
						MacAddress:          "MacAddress",
					},
					"extraNetwork": {
						IPAMConfig:          nil,
						Links:               nil,
						Aliases:             nil,
						NetworkID:           "NetworkID",
						EndpointID:          "EndpointID",
						Gateway:             "172.18.0.1",
						IPAddress:           "172.18.0.2",
						IPPrefixLen:         16,
						IPv6Gateway:         "",
						GlobalIPv6Address:   "",
						GlobalIPv6PrefixLen: 0,
						MacAddress:          "MacAddress",
					},
				},
			},
			cacheContent: types.ContainerJSON{},
			expected: []containers.NetworkAddress{
				{
					IP:       net.ParseIP("172.17.0.2"),
					Port:     80,
					Protocol: "tcp",
				},
				{
					IP:       net.ParseIP("172.18.0.2"),
					Port:     80,
					Protocol: "tcp",
				},
			},
		},
		{
			name:        "multiple ports",
			containerID: "foo",
			ports: []types.Port{
				{
					IP:          "0.0.0.0",
					PrivatePort: 8080,
					PublicPort:  8080,
					Type:        "tcp",
				},
				{
					PrivatePort: 80,
					Type:        "tcp",
				},
				{
					PrivatePort: 7000,
					Type:        "udp",
				},
			},
			netSettings: &types.SummaryNetworkSettings{
				Networks: map[string]*network.EndpointSettings{
					"bridge": {
						IPAMConfig:          nil,
						Links:               nil,
						Aliases:             nil,
						NetworkID:           "NetworkID",
						EndpointID:          "EndpointID",
						Gateway:             "172.17.0.1",
						IPAddress:           "172.17.0.2",
						IPPrefixLen:         16,
						IPv6Gateway:         "",
						GlobalIPv6Address:   "",
						GlobalIPv6PrefixLen: 0,
						MacAddress:          "MacAddress",
					},
				},
			},
			cacheContent: types.ContainerJSON{},
			expected: []containers.NetworkAddress{
				{
					IP:       net.ParseIP("0.0.0.0"),
					Port:     8080,
					Protocol: "tcp",
				},
				{
					IP:       net.ParseIP("172.17.0.2"),
					Port:     8080,
					Protocol: "tcp",
				},
				{
					IP:       net.ParseIP("172.17.0.2"),
					Port:     80,
					Protocol: "tcp",
				},
				{
					IP:       net.ParseIP("172.17.0.2"),
					Port:     7000,
					Protocol: "udp",
				},
			},
		},
		{
			name:        "multiple ports, multiple networks",
			containerID: "foo",
			ports: []types.Port{
				{
					IP:          "0.0.0.0",
					PrivatePort: 8080,
					PublicPort:  8080,
					Type:        "tcp",
				},
				{
					PrivatePort: 80,
					Type:        "tcp",
				},
				{
					PrivatePort: 7000,
					Type:        "udp",
				},
			},
			netSettings: &types.SummaryNetworkSettings{
				Networks: map[string]*network.EndpointSettings{
					"bridge": {
						IPAMConfig:          nil,
						Links:               nil,
						Aliases:             nil,
						NetworkID:           "NetworkID",
						EndpointID:          "EndpointID",
						Gateway:             "172.17.0.1",
						IPAddress:           "172.17.0.2",
						IPPrefixLen:         16,
						IPv6Gateway:         "",
						GlobalIPv6Address:   "",
						GlobalIPv6PrefixLen: 0,
						MacAddress:          "MacAddress",
					},
					"extraNetwork": {
						IPAMConfig:          nil,
						Links:               nil,
						Aliases:             nil,
						NetworkID:           "NetworkID",
						EndpointID:          "EndpointID",
						Gateway:             "172.18.0.1",
						IPAddress:           "172.18.0.2",
						IPPrefixLen:         16,
						IPv6Gateway:         "",
						GlobalIPv6Address:   "",
						GlobalIPv6PrefixLen: 0,
						MacAddress:          "MacAddress",
					},
				},
			},
			cacheContent: types.ContainerJSON{},
			expected: []containers.NetworkAddress{
				{
					IP:       net.ParseIP("0.0.0.0"),
					Port:     8080,
					Protocol: "tcp",
				},
				{
					IP:       net.ParseIP("172.17.0.2"),
					Port:     8080,
					Protocol: "tcp",
				},
				{
					IP:       net.ParseIP("172.18.0.2"),
					Port:     8080,
					Protocol: "tcp",
				},
				{
					IP:       net.ParseIP("172.17.0.2"),
					Port:     80,
					Protocol: "tcp",
				},
				{
					IP:       net.ParseIP("172.18.0.2"),
					Port:     80,
					Protocol: "tcp",
				},
				{
					IP:       net.ParseIP("172.17.0.2"),
					Port:     7000,
					Protocol: "udp",
				},
				{
					IP:       net.ParseIP("172.18.0.2"),
					Port:     7000,
					Protocol: "udp",
				},
			},
		},
	} {
		cacheKey := GetInspectCacheKey(tc.containerID, false)
		cache.Cache.Set(cacheKey, tc.cacheContent, 10*time.Second)
		d := &DockerUtil{
			cfg:            &Config{CollectNetwork: false},
			cli:            nil,
			imageNameBySha: make(map[string]string),
		}
		networkAddresses := d.parseContainerNetworkAddresses(tc.containerID, tc.ports, tc.netSettings, "mycontainer")
		assert.Len(t, networkAddresses, len(tc.expected), "test %d failed: %s", i, tc.name)
		for _, addr := range tc.expected {
			assert.Contains(t, networkAddresses, addr, "test %d failed: %s", i, tc.name)
		}
	}
}
