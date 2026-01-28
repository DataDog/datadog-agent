// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package packages

import (
	"fmt"
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
			assert.Equal(t, os.FileMode(0750), dir.Mode, fmt.Sprintf("directory %s should have mode 0750", dir.Path))
			assert.Equal(t, "dd-agent", dir.Owner, fmt.Sprintf("directory %s should have owner dd-agent", dir.Path))
			assert.Equal(t, "dd-agent", dir.Group, fmt.Sprintf("directory %s should have group dd-agent", dir.Path))
		} else {
			assert.Equal(t, os.FileMode(0755), dir.Mode, fmt.Sprintf("directory %s should have mode 0755", dir.Path))
			assert.Equal(t, "dd-agent", dir.Owner, fmt.Sprintf("directory %s should have owner dd-agent", dir.Path))
			assert.Equal(t, "dd-agent", dir.Group, fmt.Sprintf("directory %s should have group dd-agent", dir.Path))
		}
	}
}

// TestAgentPermissions tests that agent permission configurations are defined
func TestAgentPermissions(t *testing.T) {
	assert.NotEmpty(t, agentConfigPermissions, "config permissions should be defined")
	assert.NotEmpty(t, agentPackagePermissions, "package permissions should be defined")

	// Verify permissions based on actual data structure
	for _, perm := range agentConfigPermissions {
		switch perm.Path {
		case ".", "managed":
			// Base directories with recursive permissions
			assert.Equal(t, "dd-agent", perm.Owner, fmt.Sprintf("path %s should have owner dd-agent", perm.Path))
			assert.Equal(t, "dd-agent", perm.Group, fmt.Sprintf("path %s should have group dd-agent", perm.Path))
			assert.Equal(t, true, perm.Recursive, fmt.Sprintf("path %s should be recursive", perm.Path))
			assert.Equal(t, os.FileMode(0), perm.Mode, fmt.Sprintf("path %s should have default mode", perm.Path))
		case "inject", "compliance.d", "runtime-security.d":
			// Root-owned directories with recursive permissions
			assert.Equal(t, "root", perm.Owner, fmt.Sprintf("path %s should have owner root", perm.Path))
			assert.Equal(t, "root", perm.Group, fmt.Sprintf("path %s should have group root", perm.Path))
			assert.Equal(t, true, perm.Recursive, fmt.Sprintf("path %s should be recursive", perm.Path))
			assert.Equal(t, os.FileMode(0), perm.Mode, fmt.Sprintf("path %s should have default mode", perm.Path))
		case "system-probe.yaml", "security-agent.yaml", "system-probe.yaml.example", "security-agent.yaml.example":
			// Security-sensitive YAML files with restricted permissions
			assert.Equal(t, "dd-agent", perm.Owner, fmt.Sprintf("file %s should have owner dd-agent", perm.Path))
			assert.Equal(t, "dd-agent", perm.Group, fmt.Sprintf("file %s should have group dd-agent", perm.Path))
			assert.Equal(t, false, perm.Recursive, fmt.Sprintf("file %s should not be recursive", perm.Path))
			assert.Equal(t, os.FileMode(0440), perm.Mode, fmt.Sprintf("file %s should have mode 0440", perm.Path))
		}
	}
}

// TestUninstallPaths tests that uninstall paths are configured
func TestUninstallPaths(t *testing.T) {
	assert.NotEmpty(t, agentPackageUninstallPaths, "agent package uninstall paths should be defined")
	assert.NotEmpty(t, installerPackageUninstallPaths, "installer package uninstall paths should be defined")
	assert.NotEmpty(t, agentConfigUninstallPaths, "agent config uninstall paths should be defined")
}
