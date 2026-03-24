// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_rshell

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRunCommandHandlerStoresAllowedPaths(t *testing.T) {
	paths := []string{"/var/log", "/tmp"}

	handler := NewRunCommandHandler(paths)

	assert.Equal(t, paths, handler.allowedPaths)
}

func TestNewRshellBundleUsesConfiguredAllowedPaths(t *testing.T) {
	paths := []string{"/var/log", "/tmp"}

	bundle := NewRshellBundle(paths)
	action := bundle.GetAction("runCommand")

	handler, ok := action.(*RunCommandHandler)
	require.True(t, ok)
	assert.Equal(t, paths, handler.allowedPaths)
}

func TestResolveProcPathBareMetal(t *testing.T) {
	// Ensure not containerized
	t.Setenv("DOCKER_DD_AGENT", "")
	os.Unsetenv("DOCKER_DD_AGENT")

	result := resolveProcPath()

	assert.Equal(t, "/proc", result)
}

func TestResolveProcPathContainerizedWithHostMount(t *testing.T) {
	t.Setenv("DOCKER_DD_AGENT", "true")

	// /host/proc won't exist in CI, so this should fall back to /proc
	// unless running on a host where /host/proc is actually mounted
	result := resolveProcPath()

	if _, err := os.Stat("/host/proc"); err == nil {
		assert.Equal(t, "/host/proc", result)
	} else {
		assert.Equal(t, "/proc", result)
	}
}

func TestResolveProcPathContainerizedWithoutHostMount(t *testing.T) {
	// Simulate containerized environment without host mounts (e.g. Fargate)
	t.Setenv("DOCKER_DD_AGENT", "true")

	// /host/proc won't exist in test environment, so should fall back
	result := resolveProcPath()

	// In CI/test environments, /host/proc doesn't exist, so we get /proc
	if _, err := os.Stat("/host/proc"); os.IsNotExist(err) {
		assert.Equal(t, "/proc", result)
	}
}
