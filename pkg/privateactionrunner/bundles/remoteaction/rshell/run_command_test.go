// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_rshell

import (
	"errors"
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

func mockStatFn(existing map[string]bool) func(string) (os.FileInfo, error) {
	return func(path string) (os.FileInfo, error) {
		if existing[path] {
			return nil, nil
		}
		return nil, errors.New("not found")
	}
}

func overrideStatFn(t *testing.T, fn func(string) (os.FileInfo, error)) {
	original := statFn
	statFn = fn
	t.Cleanup(func() { statFn = original })
}

func TestResolveProcPathBareMetal(t *testing.T) {
	t.Setenv("DOCKER_DD_AGENT", "")
	os.Unsetenv("DOCKER_DD_AGENT")

	result := resolveProcPath()

	assert.Equal(t, "/proc", result)
}

func TestResolveProcPathContainerizedWithHostMount(t *testing.T) {
	t.Setenv("DOCKER_DD_AGENT", "true")
	overrideStatFn(t, mockStatFn(map[string]bool{"/host/proc": true}))

	result := resolveProcPath()

	assert.Equal(t, "/host/proc", result)
}

func TestResolveProcPathContainerizedWithoutHostMount(t *testing.T) {
	t.Setenv("DOCKER_DD_AGENT", "true")
	overrideStatFn(t, mockStatFn(map[string]bool{}))

	result := resolveProcPath()

	assert.Equal(t, "/proc", result)
}
