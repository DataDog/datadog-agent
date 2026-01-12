// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package packages

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestReverseStringSlice tests the reverseStringSlice function
func TestReverseStringSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "empty slice",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "single element",
			input:    []string{"a"},
			expected: []string{"a"},
		},
		{
			name:     "two elements",
			input:    []string{"a", "b"},
			expected: []string{"b", "a"},
		},
		{
			name:     "three elements",
			input:    []string{"a", "b", "c"},
			expected: []string{"c", "b", "a"},
		},
		{
			name:     "systemd units",
			input:    []string{"datadog-agent.service", "datadog-agent-trace.service", "datadog-agent-process.service"},
			expected: []string{"datadog-agent-process.service", "datadog-agent-trace.service", "datadog-agent.service"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := reverseStringSlice(tt.input)
			assert.Equal(t, tt.expected, result)
			// Verify original slice is not modified
			if len(tt.input) > 1 {
				assert.NotEqual(t, tt.input[0], result[0], "original slice should not be modified")
			}
		})
	}
}

// TestAgentDirectories tests that agent directories are properly configured
func TestAgentDirectories(t *testing.T) {
	assert.NotEmpty(t, agentDirectories)

	// Check that essential directories are included
	for _, dir := range agentDirectories {
		if dir.Path == "/var/log/datadog" {
			assert.Equal(t, os.FileMode(0750), dir.Mode)
			assert.Equal(t, "dd-agent", dir.Owner)
			assert.Equal(t, "dd-agent", dir.Group)
		} else {
			assert.Equal(t, os.FileMode(0755), dir.Mode)
			assert.Equal(t, "dd-agent", dir.Owner)
			assert.Equal(t, "dd-agent", dir.Group)
		}
	}
}

// TestAgentPermissions tests that agent permission configurations are defined
func TestAgentPermissions(t *testing.T) {
	assert.NotEmpty(t, agentConfigPermissions, "config permissions should be defined")
	assert.NotEmpty(t, agentPackagePermissions, "package permissions should be defined")

	// Verify root-owned security files have correct permissions
	for _, perm := range agentConfigPermissions {
		switch perm.Path {
		case "system-probe.yaml", "security-agent.yaml", "system-probe.yaml.example", "security-agent.yaml.example":
			assert.Equal(t, "dd-agent", perm.Owner)
			assert.Equal(t, "dd-agent", perm.Group)
			assert.Equal(t, false, perm.Recursive)
			assert.Equal(t, os.FileMode(0440), perm.Mode)
		default:
			assert.Equal(t, "root", perm.Owner)
			assert.Equal(t, "root", perm.Group)
			assert.Equal(t, true, perm.Recursive)
			assert.Equal(t, os.FileMode(0640), perm.Mode)
		}
	}
}

// TestUninstallPaths tests that uninstall paths are configured
func TestUninstallPaths(t *testing.T) {
	assert.NotEmpty(t, agentPackageUninstallPaths, "agent package uninstall paths should be defined")
	assert.NotEmpty(t, installerPackageUninstallPaths, "installer package uninstall paths should be defined")
	assert.NotEmpty(t, agentConfigUninstallPaths, "agent config uninstall paths should be defined")
}
