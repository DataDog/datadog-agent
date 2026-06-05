// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package exec

import (
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/stretchr/testify/assert"
)

func TestInstallerDirectoriesUseMsiDataDirOverride(t *testing.T) {
	t.Parallel()

	dataDir := filepath.Join(t.TempDir(), "data")
	installer := NewInstallerExec(&env.Env{
		MsiParams: env.MsiParamsEnv{
			ApplicationDataDirectory: dataDir,
		},
	}, "")

	dirs := installer.installerDirectories()

	assert.Equal(t, dataDir, dirs.DatadogDataDir)
	assert.Equal(t, filepath.Join(dataDir, "Installer"), dirs.DatadogInstallerData)
	assert.Equal(t, filepath.Join(dataDir, "Installer", "packages"), dirs.PackagesPath)
	assert.Equal(t, filepath.Join(dataDir, "Installer", "tmp"), dirs.RootTmpDir)
	assert.Equal(t, filepath.Join(dataDir, "Installer", "packages", "run"), dirs.RunPath)
}
