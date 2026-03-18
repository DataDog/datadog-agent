// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package commands

import (
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

// setupTestPaths creates a temporary directory structure that mimics the
// installer's on-disk state, and overrides paths.PackagesPath /
// paths.AgentConfigDir / paths.AgentConfigDirExp so the in-process
// getStates (Windows) reads from it.
//
// Returns a cleanup function that restores the original paths and removes
// the temporary directory.
func setupTestPaths() func() {
	origPackagesPath := paths.PackagesPath
	origAgentConfigDir := paths.AgentConfigDir
	origAgentConfigDirExp := paths.AgentConfigDirExp

	tmpDir, err := os.MkdirTemp("", "installer-test-*")
	if err != nil {
		panic(err)
	}

	packagesDir := filepath.Join(tmpDir, "packages")
	configDir := filepath.Join(tmpDir, "config")
	configDirExp := filepath.Join(tmpDir, "config-exp")

	// Create packages/datadog-agent with stable and experiment symlinks.
	// Repository.GetState reads symlinks named "stable" and "experiment"
	// and returns filepath.Base of the target as the version.
	pkgDir := filepath.Join(packagesDir, "datadog-agent")
	stableDir := filepath.Join(pkgDir, "7.31.0")
	experimentDir := filepath.Join(pkgDir, "7.32.0")
	for _, d := range []string{stableDir, experimentDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			panic(err)
		}
	}
	if err := os.Symlink(stableDir, filepath.Join(pkgDir, "stable")); err != nil {
		panic(err)
	}
	if err := os.Symlink(experimentDir, filepath.Join(pkgDir, "experiment")); err != nil {
		panic(err)
	}

	// Create config directory with .deployment-id file.
	if err := os.MkdirAll(configDir, 0755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, ".deployment-id"), []byte("abc-def-hij"), 0644); err != nil {
		panic(err)
	}

	// Create experiment config directory (no .deployment-id = empty experiment).
	if err := os.MkdirAll(configDirExp, 0755); err != nil {
		panic(err)
	}

	paths.PackagesPath = packagesDir
	paths.AgentConfigDir = configDir
	paths.AgentConfigDirExp = configDirExp

	return func() {
		paths.PackagesPath = origPackagesPath
		paths.AgentConfigDir = origAgentConfigDir
		paths.AgentConfigDirExp = origAgentConfigDirExp
		os.RemoveAll(tmpDir)
	}
}
