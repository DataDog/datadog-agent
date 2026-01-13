// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
)

// TestGetLocalInstaller tests getLocalInstaller returns a valid InstallerExec
func TestGetLocalInstaller(t *testing.T) {
	testEnv := &env.Env{}

	installer, err := getLocalInstaller(testEnv)
	require.NoError(t, err)
	assert.NotNil(t, installer)
}

// TestPackageConstants tests the package name constants
func TestPackageConstants(t *testing.T) {
	assert.Equal(t, "datadog-installer", InstallerPackage)
	assert.Equal(t, "datadog-agent", AgentPackage)
}
