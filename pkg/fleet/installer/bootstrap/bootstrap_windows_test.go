// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

func TestGetInstallPath(t *testing.T) {
	// create temp directory
	tmpDir := t.TempDir()

	// add an exe to the temp directory
	exePath := filepath.Join(tmpDir, "datadog-installer.exe")

	// touch exe PATH
	file, err := os.Create(exePath)
	if err != nil {
		t.Fatalf("Failed to create exe file: %v", err)
	}
	err = file.Close()
	if err != nil {
		t.Fatalf("Failed to close exe file: %v", err)
	}

	// get the install path
	installPath, err := getInstallerPath(t.Context(), tmpDir)
	if err != nil {
		t.Fatalf("Failed to get install path: %v", err)
	}

	// check the install path
	if installPath != exePath {
		t.Fatalf("Expected install path to be %s, got %s", exePath, installPath)
	}
}

func TestGetInstallPathSystemTemp(t *testing.T) {
	// create temp directory
	tmpDir := t.TempDir()

	// backup original CreateSystemTempDir() function and restore the original function once we're done
	// Use a different function that creates a directory in the test's temp directory
	var originalCreateSystemTempDir = paths.CreateSystemTempDir
	defer func() {
		paths.CreateSystemTempDir = originalCreateSystemTempDir
	}()

	paths.CreateSystemTempDir = func() (string, error) {
		err := os.Mkdir(filepath.Join(tmpDir, "datadog-installer"), 0755)
		if err != nil {
			return "", err
		}
		return filepath.Join(tmpDir, "datadog-installer"), nil
	}

	// add an exe to the temp directory
	exePath := filepath.Join(tmpDir, "datadog-installer.exe")

	// touch exe PATH
	file, err := os.Create(exePath)
	if err != nil {
		t.Fatalf("Failed to create exe file: %v", err)
	}
	err = file.Close()
	if err != nil {
		t.Fatalf("Failed to close exe file: %v", err)
	}

	// get the install path
	installPath, err := getInstallerPath(t.Context(), tmpDir)
	if err != nil {
		t.Fatalf("Failed to get install path: %v", err)
	}

	// check the install path
	if installPath != exePath {
		t.Fatalf("Expected install path to be %s, got %s", exePath, installPath)
	}

	// move the installer to the system temp directory
	installPath, err = moveInstallerToSystemTemp(installPath)
	if err != nil {
		t.Fatalf("Failed to move installer to system temp: %v", err)
	}

	tempPaths, err := filepath.Glob(filepath.Join(tmpDir, "datadog-installer\\*\\datadog-installer.exe"))
	if err != nil {
		t.Fatalf("Failed to glob system temp path: %v", err)
	}

	if len(tempPaths) != 1 {
		t.Fatalf("Expected 1 temp path, got %d", len(tempPaths))
	}

	if installPath != tempPaths[0] {
		t.Fatalf("Expected install path to be %s, got %s", tempPaths[0], installPath)
	}
}
