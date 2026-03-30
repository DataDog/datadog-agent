// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package exec

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/stretchr/testify/require"
)

// TestRemoveExperimentFallbackToSystemTempDir verifies that RemoveExperiment does not
// fail with "error creating temp dir" when paths.PackagesPath does not exist.
// This reproduces the scenario where the packages directory is missing (e.g. deleted
// or the installer never fully initialized) and the one-shot binary tries to recover.
func TestRemoveExperimentFallbackToSystemTempDir(t *testing.T) {
	// Override PackagesPath to a directory that does not exist.
	origPackagesPath := paths.PackagesPath
	paths.PackagesPath = filepath.Join(t.TempDir(), "nonexistent", "packages")
	defer func() { paths.PackagesPath = origPackagesPath }()

	// Provide a real file as the installer binary so that paths.CopyFile succeeds.
	fakeInstallerBin := filepath.Join(t.TempDir(), "datadog-installer.exe")
	require.NoError(t, os.WriteFile(fakeInstallerBin, []byte("fake"), 0o644))

	i := NewInstallerExec(&env.Env{}, fakeInstallerBin)
	err := i.RemoveExperiment(context.Background(), "datadog-agent")
	// The call will fail because the copied binary is not a valid executable,
	// but it must NOT fail with "error creating temp dir".
	if err != nil {
		require.NotContains(t, err.Error(), "error creating temp dir",
			"MkdirTemp should fall back to system temp dir when PackagesPath is missing")
	}
}
