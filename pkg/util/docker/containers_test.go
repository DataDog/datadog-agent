// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package docker

import (
	"net"
	"testing"

	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

func TestIsMissingIP(t *testing.T) {
	t.Run("one IP is missing", func(t *testing.T) {
		addrs := []containers.NetworkAddress{{IP: net.ParseIP("0.0.0.0")}}
		assert.True(t, isMissingIP(addrs))
	})

	t.Run("no IPs are missing", func(t *testing.T) {
		addrs := []containers.NetworkAddress{{IP: net.ParseIP("192.168.123.132")}}
		assert.False(t, isMissingIP(addrs))
	})

	t.Run("some IPs are missing", func(t *testing.T) {
		addrs := []containers.NetworkAddress{{IP: net.ParseIP("0.0.0.0")}, {IP: net.ParseIP("192.168.123.132")}}
		assert.True(t, isMissingIP(addrs))
	})
}

func TestCorrectMissingIPs(t *testing.T) {
	hostIPs := []string{"192.168.100.100", "192.168.200.200"}

	corrected := correctMissingIPs([]containers.NetworkAddress{
		{IP: net.ParseIP("0.0.0.0")},
		{IP: net.ParseIP("10.0.0.1")},
	}, hostIPs)

	assert.Equal(t, []containers.NetworkAddress{
		{IP: net.ParseIP("192.168.100.100")},
		{IP: net.ParseIP("192.168.200.200")},
		{IP: net.ParseIP("10.0.0.1")},
	}, corrected)
}

func TestCrossIPsWithPorts(t *testing.T) {
	t.Run("no IP, no port", func(t *testing.T) {
		addrs := []containers.NetworkAddress{}
		ports := []nat.Port{}
		assert.Equal(t, 0, len(crossIPsWithPorts(addrs, ports)))
	})

	t.Run("one IP, no port", func(t *testing.T) {
		addrs := []containers.NetworkAddress{{IP: net.ParseIP("192.168.123.132")}}
		ports := []nat.Port{}
		assert.Equal(t, 1, len(crossIPsWithPorts(addrs, ports)))
		assert.Equal(t, addrs, crossIPsWithPorts(addrs, ports))
	})

	t.Run("no IP, one port", func(t *testing.T) {
		addrs := []containers.NetworkAddress{}
		ports := []nat.Port{"1337/tcp"}
		assert.Equal(t, 0, len(crossIPsWithPorts(addrs, ports)))
	})

	t.Run("one IP, one port", func(t *testing.T) {
		addrs := []containers.NetworkAddress{{IP: net.ParseIP("192.168.123.132")}}
		ports := []nat.Port{"1337/tcp"}
		res := []containers.NetworkAddress{{
			IP:       net.ParseIP("192.168.123.132"),
			Port:     1337,
			Protocol: "tcp",
		}}
		assert.Equal(t, 1, len(crossIPsWithPorts(addrs, ports)))
		assert.Equal(t, res, crossIPsWithPorts(addrs, ports))
	})

	t.Run("two IP, one port", func(t *testing.T) {
		addrs := []containers.NetworkAddress{
			{IP: net.ParseIP("192.168.123.132")},
			{IP: net.ParseIP("172.17.0.3")},
		}
		ports := []nat.Port{"1337/tcp"}
		addr1 := containers.NetworkAddress{
			IP:       net.ParseIP("192.168.123.132"),
			Port:     1337,
			Protocol: "tcp",
		}
		addr2 := containers.NetworkAddress{
			IP:       net.ParseIP("172.17.0.3"),
			Port:     1337,
			Protocol: "tcp",
		}
		assert.Equal(t, 2, len(crossIPsWithPorts(addrs, ports)))
		assert.Contains(t, crossIPsWithPorts(addrs, ports), addr1)
		assert.Contains(t, crossIPsWithPorts(addrs, ports), addr2)
	})

	t.Run("one IP, two port", func(t *testing.T) {
		addrs := []containers.NetworkAddress{
			{IP: net.ParseIP("172.17.0.3")},
		}
		ports := []nat.Port{"1337/tcp", "1338/udp"}
		addr1 := containers.NetworkAddress{
			IP:       net.ParseIP("172.17.0.3"),
			Port:     1337,
			Protocol: "tcp",
		}
		addr2 := containers.NetworkAddress{
			IP:       net.ParseIP("172.17.0.3"),
			Port:     1338,
			Protocol: "udp",
		}
		assert.Equal(t, 2, len(crossIPsWithPorts(addrs, ports)))
		assert.Contains(t, crossIPsWithPorts(addrs, ports), addr1)
		assert.Contains(t, crossIPsWithPorts(addrs, ports), addr2)
	})

	t.Run("two IP, two port", func(t *testing.T) {
		addrs := []containers.NetworkAddress{
			{IP: net.ParseIP("192.168.123.132")},
			{IP: net.ParseIP("172.17.0.3")},
		}
		ports := []nat.Port{"1337/tcp", "1338/udp"}
		addr1 := containers.NetworkAddress{
			IP:       net.ParseIP("172.17.0.3"),
			Port:     1337,
			Protocol: "tcp",
		}
		addr2 := containers.NetworkAddress{
			IP:       net.ParseIP("172.17.0.3"),
			Port:     1338,
			Protocol: "udp",
		}
		addr3 := containers.NetworkAddress{
			IP:       net.ParseIP("192.168.123.132"),
			Port:     1337,
			Protocol: "tcp",
		}
		addr4 := containers.NetworkAddress{
			IP:       net.ParseIP("192.168.123.132"),
			Port:     1338,
			Protocol: "udp",
		}
		assert.Equal(t, 4, len(crossIPsWithPorts(addrs, ports)))
		assert.Contains(t, crossIPsWithPorts(addrs, ports), addr1)
		assert.Contains(t, crossIPsWithPorts(addrs, ports), addr2)
		assert.Contains(t, crossIPsWithPorts(addrs, ports), addr3)
		assert.Contains(t, crossIPsWithPorts(addrs, ports), addr4)
	})
}
