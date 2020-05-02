// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package gohai

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetPayload(t *testing.T) {
	gohai := GetPayload()

	assert.NotNil(t, gohai.Gohai.CPU)
	assert.NotNil(t, gohai.Gohai.FileSystem)
	assert.NotNil(t, gohai.Gohai.Memory)
	assert.NotNil(t, gohai.Gohai.Network)
	assert.NotNil(t, gohai.Gohai.Platform)
}

func TestGetPayloadContainerized(t *testing.T) {
	os.Setenv("DOCKER_DD_AGENT", "true")
	defer os.Unsetenv("DOCKER_DD_AGENT")

	detectDocker0()
	oldDocker0Detected := docker0Detected
	docker0Detected = false
	defer func() { docker0Detected = oldDocker0Detected }()

	gohai := GetPayload()

	assert.NotNil(t, gohai.Gohai.CPU)
	assert.NotNil(t, gohai.Gohai.FileSystem)
	assert.NotNil(t, gohai.Gohai.Memory)
	assert.Nil(t, gohai.Gohai.Network)
	assert.NotNil(t, gohai.Gohai.Platform)
}

func TestGetPayloadContainerizedWithDocker0(t *testing.T) {
	os.Setenv("DOCKER_DD_AGENT", "true")
	defer os.Unsetenv("DOCKER_DD_AGENT")

	detectDocker0()
	oldDocker0Detected := docker0Detected
	docker0Detected = true
	defer func() { docker0Detected = oldDocker0Detected }()

	gohai := GetPayload()

	assert.NotNil(t, gohai.Gohai.CPU)
	assert.NotNil(t, gohai.Gohai.FileSystem)
	assert.NotNil(t, gohai.Gohai.Memory)
	assert.NotNil(t, gohai.Gohai.Network)
	assert.NotNil(t, gohai.Gohai.Platform)
}
