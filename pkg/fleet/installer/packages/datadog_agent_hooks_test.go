// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package packages

import (
	"os"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/file"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIsAmbiantCapabilitiesSupported tests the ambient capabilities detection
func TestIsAmbiantCapabilitiesSupported(t *testing.T) {
	// This test will work on Linux systems with /proc/self/status
	supported, err := isAmbiantCapabilitiesSupported()
	require.NoError(t, err)

	// Read the actual status file to verify
	content, err := os.ReadFile("/proc/self/status")
	require.NoError(t, err)

	expectedSupported := strings.Contains(string(content), "CapAmb:")
	assert.Equal(t, expectedSupported, supported, "ambient capabilities detection should match actual presence in /proc/self/status")
}

// TestHooksRegistration tests that hooks are properly registered
func TestHooksRegistration(t *testing.T) {
	hooks := datadogAgentPackage

	t.Run("installation hooks registered", func(t *testing.T) {
		assert.NotNil(t, hooks.preInstall, "preInstall hook should be registered")
		assert.NotNil(t, hooks.postInstall, "postInstall hook should be registered")
		assert.NotNil(t, hooks.preRemove, "preRemove hook should be registered")
	})

	t.Run("experiment hooks registered", func(t *testing.T) {
		assert.NotNil(t, hooks.preStartExperiment, "preStartExperiment hook should be registered")
		assert.NotNil(t, hooks.postStartExperiment, "postStartExperiment hook should be registered")
		assert.NotNil(t, hooks.preStopExperiment, "preStopExperiment hook should be registered")
		assert.NotNil(t, hooks.prePromoteExperiment, "prePromoteExperiment hook should be registered")
		assert.NotNil(t, hooks.postPromoteExperiment, "postPromoteExperiment hook should be registered")
	})

	t.Run("config experiment hooks registered", func(t *testing.T) {
		assert.NotNil(t, hooks.postStartConfigExperiment, "postStartConfigExperiment hook should be registered")
		assert.NotNil(t, hooks.preStopConfigExperiment, "preStopConfigExperiment hook should be registered")
		assert.NotNil(t, hooks.postPromoteConfigExperiment, "postPromoteConfigExperiment hook should be registered")
	})
}

// TestOldInstallerUnitPaths tests the old installer unit paths configuration
func TestOldInstallerUnitPaths(t *testing.T) {
	assert.NotEmpty(t, oldInstallerUnitPaths)

	// Verify expected old installer units are in the list
	expectedPaths := []file.Path{
		"datadog-installer-exp.service",
		"datadog-installer.service",
	}

	for _, path := range expectedPaths {
		assert.Contains(t, oldInstallerUnitPaths, path, "old installer unit paths should include %s", path)
	}
}

// TestRootOwnedDirectories tests that certain directories are root-owned for security
func TestRootOwnedDirectories(t *testing.T) {
	rootOwnedPaths := []string{
		"inject",
		"compliance.d",
		"runtime-security.d",
	}

	for _, perm := range agentConfigPermissions {
		for _, rootPath := range rootOwnedPaths {
			if perm.Path == rootPath {
				t.Run(rootPath, func(t *testing.T) {
					assert.Equal(t, "root", perm.Owner,
						"directory %s should be owned by root", rootPath)
					assert.Equal(t, "root", perm.Group,
						"directory %s should have group root", rootPath)
					assert.True(t, perm.Recursive,
						"directory %s should have recursive permissions", rootPath)
				})
			}
		}
	}
}
