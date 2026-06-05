// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package paths defines commonly used paths throughout the installer
package paths

import (
	"fmt"
	"io"
	"os"
)

// EnsureInstallerDirectories creates the installer data, packages, configs, tmp,
// and run directories if they do not exist.
func EnsureInstallerDirectories() error {
	err := EnsureInstallerDataDir()
	if err != nil {
		return fmt.Errorf("could not ensure installer data directory permissions: %w", err)
	}

	return ensureInstallerSubdirectories(PackagesPath, ConfigsPath, RootTmpDir, RunPath)
}

func ensureInstallerSubdirectories(packagesPath, configsPath, rootTmpDir, runPath string) error {
	err := os.MkdirAll(packagesPath, 0755)
	if err != nil {
		return fmt.Errorf("error creating packages directory: %w", err)
	}
	err = os.MkdirAll(configsPath, 0755)
	if err != nil {
		return fmt.Errorf("error creating configs directory: %w", err)
	}
	err = os.MkdirAll(rootTmpDir, 0755)
	if err != nil {
		return fmt.Errorf("error creating tmp directory: %w", err)
	}
	err = os.MkdirAll(runPath, 0755)
	if err != nil {
		return fmt.Errorf("error creating run directory: %w", err)
	}

	return nil
}

// CopyFile copies a file from src to dst
func CopyFile(src, dst string) error {
	// Open the source file
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("could not open source file: %w", err)
	}
	defer sourceFile.Close()

	// Create the destination file
	destinationFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("could not create destination file: %w", err)
	}
	defer destinationFile.Close()

	// Copy the contents from source to destination
	_, err = io.Copy(destinationFile, sourceFile)
	if err != nil {
		return fmt.Errorf("could not copy file: %w", err)
	}

	// Flush the destination file to ensure all data is written
	err = destinationFile.Sync()
	if err != nil {
		return fmt.Errorf("could not flush destination file: %w", err)
	}

	return nil
}
