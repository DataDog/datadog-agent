// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_rshell

import (
	"os"
	"path/filepath"
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
	t.Setenv("DOCKER_DD_AGENT", "")
	os.Unsetenv("DOCKER_DD_AGENT")

	result := resolveProcPath()

	assert.Equal(t, "/proc", result)
}

func TestResolveProcPathContainerizedWithHostMount(t *testing.T) {
	t.Setenv("DOCKER_DD_AGENT", "true")

	// Point prefix at a temp dir with a proc directory to simulate /host/proc
	tmpDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "proc"), 0755))

	original := containerizedPathPrefix
	containerizedPathPrefix = tmpDir
	t.Cleanup(func() { containerizedPathPrefix = original })

	result := resolveProcPath()

	assert.Equal(t, filepath.Join(tmpDir, "proc"), result)
}

func TestResolveProcPathContainerizedWithoutHostMount(t *testing.T) {
	t.Setenv("DOCKER_DD_AGENT", "true")

	// Point prefix at an empty temp dir — no proc mount exists
	tmpDir := t.TempDir()

	original := containerizedPathPrefix
	containerizedPathPrefix = tmpDir
	t.Cleanup(func() { containerizedPathPrefix = original })

	result := resolveProcPath()

	assert.Equal(t, "/proc", result)
}
