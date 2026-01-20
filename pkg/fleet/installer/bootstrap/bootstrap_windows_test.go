// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package bootstrap

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
)

// TestGetInstallerOCIWithDefaultVersion tests getInstallerOCI with default version on Windows
func TestGetInstallerOCIWithDefaultVersion(t *testing.T) {
	ctx := context.Background()
	testEnv := &env.Env{}

	url, err := getInstallerOCI(ctx, testEnv)
	require.NoError(t, err)
	// PackageURL strips "datadog-" prefix and adds "-package" suffix
	// On Windows, AgentPackage is "datadog-agent" which becomes "agent-package"
	assert.Contains(t, url, "agent-package")
	assert.Contains(t, url, "latest")
	assert.Contains(t, url, "oci://")
}

// TestGetInstallerOCIWithVersionOverride tests getInstallerOCI with version override on Windows
func TestGetInstallerOCIWithVersionOverride(t *testing.T) {
	ctx := context.Background()
	testEnv := &env.Env{
		DefaultPackagesVersionOverride: map[string]string{"datadog-agent": "7.65.0"},
	}

	url, err := getInstallerOCI(ctx, testEnv)
	require.NoError(t, err)
	assert.Contains(t, url, "agent-package")
	assert.Contains(t, url, "7.65.0")
	assert.NotContains(t, url, "latest")
}

// TestGetInstallerOCIWithDifferentSite tests getInstallerOCI with different site on Windows
func TestGetInstallerOCIWithDifferentSite(t *testing.T) {
	ctx := context.Background()
	testEnv := &env.Env{
		Site: "datad0g.com",
	}

	url, err := getInstallerOCI(ctx, testEnv)
	require.NoError(t, err)
	assert.Contains(t, url, "agent-package")
	assert.Contains(t, url, "datad0g.com")
}

// TestGetInstallerOCIWithUnsupportedVersion tests getInstallerOCI rejects unsupported versions on Windows
func TestGetInstallerOCIWithUnsupportedVersion(t *testing.T) {
	ctx := context.Background()
	testEnv := &env.Env{
		AgentMajorVersion: "7",
		AgentMinorVersion: "50.0", // Below minimum version 7.65.0
	}

	_, err := getInstallerOCI(ctx, testEnv)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support fleet automation")
}
