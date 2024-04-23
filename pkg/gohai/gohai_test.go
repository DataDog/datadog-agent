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
	gohai := GetPayload(false)

	assert.Nil(t, gohai.Gohai.CPU)
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

	gohai := GetPayload(true)

	assert.NotNil(t, gohai.Gohai.CPU)
	assert.NotNil(t, gohai.Gohai.FileSystem)
	assert.NotNil(t, gohai.Gohai.Memory)
	assert.Nil(t, gohai.Gohai.Network)
	assert.NotNil(t, gohai.Gohai.Platform)
}

func TestGetPayloadContainerizedWithDocker0(t *testing.T) {
	t.Setenv("DOCKER_DD_AGENT", "true")

	detectDocker0()
	oldDocker0Detected := docker0Detected
	docker0Detected = true
	defer func() { docker0Detected = oldDocker0Detected }()

	gohai := GetPayload(false)

	assert.NotNil(t, gohai.Gohai.CPU)
	assert.NotNil(t, gohai.Gohai.FileSystem)
	assert.NotNil(t, gohai.Gohai.Memory)
	assert.NotNil(t, gohai.Gohai.Network)
	assert.NotNil(t, gohai.Gohai.Platform)
}
