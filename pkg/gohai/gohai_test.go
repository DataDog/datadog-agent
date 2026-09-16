// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gohai

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetPayload(t *testing.T) {
	gohai := GetPayload("hostname", false, false, "")
	assert.NotNil(t, gohai.Gohai.CPU)
	assert.NotNil(t, gohai.Gohai.FileSystem)
	assert.NotNil(t, gohai.Gohai.Memory)
	assert.NotNil(t, gohai.Gohai.Network)
	assert.NotNil(t, gohai.Gohai.Platform)

	gohai = GetPayload("hostname", true, false, "")
	assert.NotNil(t, gohai.Gohai.CPU)
	assert.NotNil(t, gohai.Gohai.FileSystem)
	assert.NotNil(t, gohai.Gohai.Memory)
	assert.NotNil(t, gohai.Gohai.Network)
	assert.NotNil(t, gohai.Gohai.Platform)
}

func TestGetPayloadContainerized(t *testing.T) {
	t.Setenv("DOCKER_DD_AGENT", "true")

	detectDocker0()
	oldDocker0Detected := docker0Detected
	docker0Detected = false
	defer func() { docker0Detected = oldDocker0Detected }()

	gohai := GetPayload("hostname", false, true, "")
	assert.NotNil(t, gohai.Gohai.CPU)
	assert.NotNil(t, gohai.Gohai.FileSystem)
	assert.NotNil(t, gohai.Gohai.Memory)
	assert.Nil(t, gohai.Gohai.Network)
	assert.NotNil(t, gohai.Gohai.Platform)

	gohai = GetPayload("hostname", true, true, "")
	assert.NotNil(t, gohai.Gohai.CPU)
	assert.NotNil(t, gohai.Gohai.FileSystem)
	assert.NotNil(t, gohai.Gohai.Memory)
	assert.Nil(t, gohai.Gohai.Network)
	assert.NotNil(t, gohai.Gohai.Platform)
}

func TestGetPayloadContainerizedWithFallbackHost(t *testing.T) {
	t.Setenv("DOCKER_DD_AGENT", "true")

	detectDocker0()
	oldDocker0Detected := docker0Detected
	docker0Detected = false
	defer func() { docker0Detected = oldDocker0Detected }()

	gohai := GetPayload("hostname", false, true, "10.0.1.23")
	assert.NotNil(t, gohai.Gohai.CPU)
	assert.NotNil(t, gohai.Gohai.FileSystem)
	assert.NotNil(t, gohai.Gohai.Memory)
	assert.NotNil(t, gohai.Gohai.Platform)

	network, ok := gohai.Gohai.Network.(map[string]interface{})
	assert.True(t, ok, "expected fallback network payload to be a map")
	assert.Equal(t, "10.0.1.23", network["ipaddress"])
}

func TestGetPayloadContainerizedWithUnresolvableFallbackHost(t *testing.T) {
	t.Setenv("DOCKER_DD_AGENT", "true")

	detectDocker0()
	oldDocker0Detected := docker0Detected
	docker0Detected = false
	defer func() { docker0Detected = oldDocker0Detected }()

	// Neither a valid literal nor a resolvable hostname: must not be written into a
	// field documented as an IPv4 address, and must not fall back to reporting nothing
	// silently wrong either - Network should simply stay absent.
	gohai := GetPayload("hostname", false, true, "this-hostname-does-not-resolve.invalid")
	assert.Nil(t, gohai.Gohai.Network)
}

func TestResolveFallbackHostIPv4(t *testing.T) {
	// valid IPv4 literal passes through unchanged
	assert.Equal(t, "10.0.1.23", resolveFallbackHostIPv4("10.0.1.23"))

	// loopback literal is rejected, even though it's a valid IPv4
	assert.Equal(t, "", resolveFallbackHostIPv4("127.0.0.1"))

	// IPv6 literal is rejected (network.ipaddress is documented as IPv4)
	assert.Equal(t, "", resolveFallbackHostIPv4("2001:db8::1"))

	// hostname that resolves to loopback (e.g. "localhost") is rejected
	assert.Equal(t, "", resolveFallbackHostIPv4("localhost"))

	// empty input is a no-op
	assert.Equal(t, "", resolveFallbackHostIPv4(""))

	// hostname that doesn't resolve at all is rejected, not passed through raw
	assert.Equal(t, "", resolveFallbackHostIPv4("this-hostname-does-not-resolve.invalid"))
}

func TestGetPayloadContainerizedWithDocker0(t *testing.T) {
	t.Setenv("DOCKER_DD_AGENT", "true")

	detectDocker0()
	oldDocker0Detected := docker0Detected
	docker0Detected = true
	defer func() { docker0Detected = oldDocker0Detected }()

	gohai := GetPayload("hostname", false, false, "")
	assert.NotNil(t, gohai.Gohai.CPU)
	assert.NotNil(t, gohai.Gohai.FileSystem)
	assert.NotNil(t, gohai.Gohai.Memory)
	assert.NotNil(t, gohai.Gohai.Network)
	assert.NotNil(t, gohai.Gohai.Platform)

	gohai = GetPayload("hostname", true, false, "")
	assert.NotNil(t, gohai.Gohai.CPU)
	assert.NotNil(t, gohai.Gohai.FileSystem)
	assert.NotNil(t, gohai.Gohai.Memory)
	assert.NotNil(t, gohai.Gohai.Network)
	assert.NotNil(t, gohai.Gohai.Platform)
}
