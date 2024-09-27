// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package secretsimpl

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestGetExecutablePermissionsError(t *testing.T) {
	tel := fxutil.Test[telemetry.Component](t, nooptelemetry.Module())
	resolver := newEnabledSecretResolver(tel)
	resolver.backendCommand = "some_command"

	res, err := resolver.getExecutablePermissions()
	require.NoError(t, err)
	require.IsType(t, permissionsDetails{}, res)
	details := res.(permissionsDetails)
	assert.Equal(t, "Error calling 'get-acl': exit status 1", details.Error)
	assert.Equal(t, "", details.Stdout)
	assert.NotEqual(t, "", details.Stderr)
}

func setupSecretCommmand(t *testing.T, resolver *secretResolver) {
	dir := t.TempDir()

	resolver.backendCommand = filepath.Join(dir, "an executable with space")
	f, err := os.Create(resolver.backendCommand)
	require.NoError(t, err)
	f.Close()

	exec.Command("powershell", "test/setAcl.ps1",
		"-file", fmt.Sprintf("\"%s\"", resolver.backendCommand),
		"-removeAllUser", "0",
		"-removeAdmin", "0",
		"-removeLocalSystem", "0",
		"-addDDuser", "1").Run()
}

func TestGetExecutablePermissionsSuccess(t *testing.T) {
	tel := fxutil.Test[telemetry.Component](t, nooptelemetry.Module())
	resolver := newEnabledSecretResolver(tel)
	setupSecretCommmand(t, resolver)

	res, err := resolver.getExecutablePermissions()
	require.NoError(t, err)
	require.IsType(t, permissionsDetails{}, res)
	details := res.(permissionsDetails)
	assert.Equal(t, "", details.Error)
	assert.NotEqual(t, "", details.Stdout)
	assert.Equal(t, "", details.Stderr)
}
