// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShortContainerID(t *testing.T) {
	var containerID string

	containerID = "abcdefghijklmnopqrstuvwxyz"
	assert.Equal(t, "abcdefghijkl", ShortContainerID(containerID))

	containerID = "abcde"
	assert.Equal(t, "abcde", ShortContainerID(containerID))
}
