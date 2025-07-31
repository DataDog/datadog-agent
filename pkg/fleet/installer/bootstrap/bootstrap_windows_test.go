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

	// check the install path is in the system temp directory
	systemTempPath := filepath.Join(os.TempDir(), "datadog-installer.exe")

	installPath, err = moveInstallerToSystemTemp(installPath)
	if err != nil {
		t.Fatalf("Failed to move installer to system temp: %v", err)
	}

	// check the install path is in the system temp directory
	if installPath != systemTempPath {
		t.Fatalf("Expected install path to be %s, got %s", systemTempPath, installPath)
	}
}
