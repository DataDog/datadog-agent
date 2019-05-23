// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package docker

import (
	"net"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/network"
	"github.com/stretchr/testify/assert"
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

func TestParseContainerNetworkAddresses(t *testing.T) {
	ports := []types.Port{
		{
			IP:          "0.0.0.0",
			PrivatePort: 80,
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
	}
	netSettings := &types.SummaryNetworkSettings{
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
			"network1": {
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
	}
	expectedOutput := []containers.NetworkAddress{
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
		{
			IP:       net.ParseIP("172.17.0.2"),
			Port:     7000,
			Protocol: "udp",
		},
		{
			IP:       net.ParseIP("172.18.0.2"),
			Port:     80,
			Protocol: "tcp",
		},
		{
			IP:       net.ParseIP("172.18.0.2"),
			Port:     7000,
			Protocol: "udp",
		},
	}
	result := parseContainerNetworkAddresses(ports, netSettings, "mycontainer")

	// Cannot use assert.Equal because the order of elements in result is random
	assert.Len(t, result, len(expectedOutput))
	for _, addr := range expectedOutput {
		assert.Contains(t, result, addr)
	}
}
