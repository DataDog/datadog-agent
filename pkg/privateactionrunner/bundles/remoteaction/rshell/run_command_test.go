// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_rshell

import (
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
