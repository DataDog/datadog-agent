// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDirectories_GetState(t *testing.T) {
	tmpDir := t.TempDir()
	stablePath := filepath.Join(tmpDir, "stable")
	experimentPath := filepath.Join(tmpDir, "experiment")

	err := os.MkdirAll(stablePath, 0755)
	assert.NoError(t, err)
	err = os.MkdirAll(experimentPath, 0755)
	assert.NoError(t, err)

	dirs := &Directories{
		StablePath:     stablePath,
		ExperimentPath: experimentPath,
	}

	// Test with no deployment IDs
	state, err := dirs.GetState()
	assert.NoError(t, err)
	assert.Equal(t, "", state.StableDeploymentID)
	assert.Equal(t, "", state.ExperimentDeploymentID)

	// Test with stable deployment ID only
	err = os.WriteFile(filepath.Join(stablePath, deploymentIDFile), []byte("stable-123"), 0644)
	assert.NoError(t, err)

	state, err = dirs.GetState()
	assert.NoError(t, err)
	assert.Equal(t, "stable-123", state.StableDeploymentID)
	assert.Equal(t, "", state.ExperimentDeploymentID)

	// Test with both deployment IDs
	err = os.WriteFile(filepath.Join(experimentPath, deploymentIDFile), []byte("experiment-456"), 0644)
	assert.NoError(t, err)

	state, err = dirs.GetState()
	assert.NoError(t, err)
	assert.Equal(t, "stable-123", state.StableDeploymentID)
	assert.Equal(t, "experiment-456", state.ExperimentDeploymentID)

	// Test with symlinked experiment (should clear experiment deployment ID)
	err = os.Remove(filepath.Join(experimentPath, deploymentIDFile))
	assert.NoError(t, err)
	err = os.Symlink(filepath.Join(stablePath, deploymentIDFile), filepath.Join(experimentPath, deploymentIDFile))
	assert.NoError(t, err)

	state, err = dirs.GetState()
	assert.NoError(t, err)
	assert.Equal(t, "stable-123", state.StableDeploymentID)
	assert.Equal(t, "", state.ExperimentDeploymentID)
}
