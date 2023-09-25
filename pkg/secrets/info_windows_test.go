// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build secrets && windows

package secrets

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetExecutablePermissionsError(t *testing.T) {
	// This test plus unit tests for Linux file will be moving to integration tests
	// (in soon to be added _windows_integration_tests() and _linux_integration_tests() functions)
	t.Skip("skipping flaky Windows test (it hangs on execution of its PowerShell script in latest github environment)")

	secretBackendCommand = "some_command"
	t.Cleanup(resetPackageVars)

	res, err := getExecutablePermissions()
	require.NoError(t, err)
	require.IsType(t, permissionsDetails{}, res)
	details := res.(permissionsDetails)
	assert.Equal(t, "Error calling 'get-acl': exit status 1", details.Error)
	assert.Equal(t, "", details.Stdout)
	assert.NotEqual(t, "", details.Stderr)
}

func setupSecretCommmand(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(resetPackageVars)

	secretBackendCommand = filepath.Join(dir, "an executable with space")
	f, err := os.Create(secretBackendCommand)
	require.NoError(t, err)
	f.Close()

	exec.Command("powershell", "test/setAcl.ps1",
		"-file", fmt.Sprintf("\"%s\"", secretBackendCommand),
		"-removeAllUser", "0",
		"-removeAdmin", "0",
		"-removeLocalSystem", "0",
		"-addDDuser", "1").Run()
}

func TestGetExecutablePermissionsSuccess(t *testing.T) {
	// This test plus unit tests for Linux file will be moving to integration tests
	// (in soon to be added _windows_integration_tests() and _linux_integration_tests() functions)
	t.Skip("skipping flaky Windows test (it hangs on execution of its PowerShell script in latest github environment)")

	setupSecretCommmand(t)

	res, err := getExecutablePermissions()
	require.NoError(t, err)
	require.IsType(t, permissionsDetails{}, res)
	details := res.(permissionsDetails)
	assert.Equal(t, "", details.Error)
	assert.NotEqual(t, "", details.Stdout)
	assert.Equal(t, "", details.Stderr)
}
