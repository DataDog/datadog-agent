// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package exec

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/config"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
)

// writeFakeInstaller writes an executable stand-in for the installer binary that
// consumes stdin (the secrets payload), writes the given stderr output, and exits
// with the given code. The stderr payload is passed via a file to avoid any shell
// quoting concerns.
func writeFakeInstaller(t *testing.T, stderr string, exitCode int) string {
	t.Helper()
	dir := t.TempDir()
	stderrPath := filepath.Join(dir, "stderr.txt")
	require.NoError(t, os.WriteFile(stderrPath, []byte(stderr), 0o644))
	path := filepath.Join(dir, "fake-installer.sh")
	script := "#!/bin/sh\n" +
		"cat >/dev/null\n" + // drain stdin so the parent's write doesn't get EPIPE
		"cat " + stderrPath + " 1>&2\n" +
		"exit " + strconv.Itoa(exitCode) + "\n"
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path
}

// TestInstallConfigExperiment_PropagatesInstallerError guarantees the fix for the
// bare "exit status 255" problem: the installer child's error (which names the
// offending config file and the YAML parse failure) must be surfaced in the error
// returned to the daemon, not swallowed.
func TestInstallConfigExperiment_PropagatesInstallerError(t *testing.T) {
	childErrJSON := `{"error":"could not write experiment: could not parse config file \"/application_monitoring.yaml\" as YAML (fix the syntax error reported below and retry): yaml: line 17: did not find expected key","code":0}`
	binPath := writeFakeInstaller(t, childErrJSON, 255)

	i := NewInstallerExec(&env.Env{}, binPath)
	err := i.InstallConfigExperiment(context.Background(), "datadog-agent", config.Operations{
		DeploymentID: "test",
	}, map[string]string{"secret": "value"})

	require.Error(t, err)
	// The reconstructed installer error must name the file and preserve the parse error.
	assert.ErrorContains(t, err, "/application_monitoring.yaml")
	assert.ErrorContains(t, err, "did not find expected key")
	// The raw exit status is still present for debugging, but no longer the only signal.
	assert.ErrorContains(t, err, "exit status 255")
}

// TestInstallConfigExperiment_FallsBackToExitStatus verifies that when the child
// writes nothing to stderr we still return the raw exit error rather than an empty one.
func TestInstallConfigExperiment_FallsBackToExitStatus(t *testing.T) {
	binPath := writeFakeInstaller(t, "", 255)

	i := NewInstallerExec(&env.Env{}, binPath)
	err := i.InstallConfigExperiment(context.Background(), "datadog-agent", config.Operations{}, nil)

	require.Error(t, err)
	assert.ErrorContains(t, err, "exit status 255")
}

// TestInstallConfigExperiment_Success verifies the happy path still returns nil.
func TestInstallConfigExperiment_Success(t *testing.T) {
	binPath := writeFakeInstaller(t, "", 0)

	i := NewInstallerExec(&env.Env{}, binPath)
	err := i.InstallConfigExperiment(context.Background(), "datadog-agent", config.Operations{}, nil)

	assert.NoError(t, err)
}
