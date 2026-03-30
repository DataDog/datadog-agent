// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package secretsimpl

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

func TestGetExecutablePermissionsError(t *testing.T) {
	tel := nooptelemetry.GetCompatComponent()
	resolver := newEnabledSecretResolver(tel)
	resolver.backendCommand = "some_command"

	res, err := resolver.getExecutablePermissions()
	require.NoError(t, err)
	assert.Equal(t, "Error calling 'get-acl': exit status 1", res.Error)
	assert.Equal(t, "", res.StdOut)
	assert.NotEqual(t, "", res.StdErr)
}

func setupSecretCommmand(t *testing.T, resolver *secretResolver) {
	dir := t.TempDir()

	resolver.backendCommand = filepath.Join(dir, "an executable with space")
	f, err := os.Create(resolver.backendCommand)
	require.NoError(t, err)
	f.Close()

	filesystem.SetACL(fmt.Sprintf("%q", resolver.backendCommand), false, false, false, true)
}

func TestGetExecutablePermissionsSuccess(t *testing.T) {
	tel := nooptelemetry.GetCompatComponent()
	resolver := newEnabledSecretResolver(tel)
	setupSecretCommmand(t, resolver)

	res, err := resolver.getExecutablePermissions()
	require.NoError(t, err)

	assert.Equal(t, "", res.Error)
	assert.NotEqual(t, "", res.StdOut)
	assert.Equal(t, "", res.StdErr)
}
