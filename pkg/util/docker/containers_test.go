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
	"github.com/stretchr/testify/assert"
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
