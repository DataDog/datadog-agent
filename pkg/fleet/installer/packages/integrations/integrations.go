// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package integrations contains packaging logic for python integrations
package integrations

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

var (
	datadogInstalledIntegrationsPattern = regexp.MustCompile(`embedded/lib/python[^/]+/site-packages/datadog_.*`)
)

// SaveCustomIntegrations saves custom integrations from the previous installation
// Today it calls pre.py to persist the custom integrations; though we should probably
// port this to Go in the future.
//
// Note: in the OCI installation this fails as the file where integrations are saved
// is hardcoded to be in the same directory as the agent. This will be fixed in a future PR.
func SaveCustomIntegrations(ctx context.Context, installPath string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "save_custom_integrations")
	defer func() {
		span.Finish(err)
	}()

	storagePath := installPath
	if strings.HasPrefix(installPath, paths.PackagesPath) {
		storagePath = paths.RootTmpDir
	}

	if _, err := os.Stat(filepath.Join(installPath, "embedded/bin/python")); err == nil {
		cmd := exec.CommandContext(ctx, filepath.Join(installPath, "embedded/bin/python"), filepath.Join(installPath, "python-scripts/pre.py"), installPath, storagePath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to run integration persistence in pre.py: %w", err)
		}
	}
	return nil
}

// RestoreCustomIntegrations restores custom integrations from the previous installation
// Today it calls post.py to persist the custom integrations; though we should probably
// port this to Go in the future.
//
// Note: in the OCI installation this fails as the file where integrations are saved
// is hardcoded to be in the same directory as the agent. This will be fixed in a future PR.
func RestoreCustomIntegrations(ctx context.Context, installPath string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "restore_custom_integrations")
	defer func() {
		span.Finish(err)
	}()

	storagePath := installPath
	if strings.HasPrefix(installPath, paths.PackagesPath) {
		storagePath = paths.RootTmpDir
	}

	if _, err := os.Stat(filepath.Join(installPath, "embedded/bin/python")); err == nil {
		cmd := exec.CommandContext(ctx, filepath.Join(installPath, "embedded/bin/python"), filepath.Join(installPath, "python-scripts/post.py"), installPath, storagePath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to run integration persistence in post.py: %w", err)
		}
	}
	return nil
}

// getAllIntegrations retrieves all integration paths installed by the package
// It walks through the installPath and collects paths that match the './embedded/lib/python*/site-packages/datadog_*' pattern.
func getAllIntegrations(installPath string) ([]string, error) {
	allIntegrations := make([]string, 0)
	err := filepath.Walk(installPath, func(path string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if datadogInstalledIntegrationsPattern.MatchString(path) {
			allIntegrations = append(allIntegrations, path) // Absolute path
		}
		return nil
	})
	return allIntegrations, err
}

// RemoveCustomIntegrations removes custom integrations that are not installed by the package
//
// Since 6.18.0, a file containing all integrations files which have been installed by
// the package is available. We use it to remove only the datadog-related check files which
// have *NOT* been installed by the package (eg: installed using the `integration` command).
func RemoveCustomIntegrations(ctx context.Context, installPath string) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "remove_custom_integrations")
	defer func() { span.Finish(err) }()

	if _, err := os.Stat(filepath.Join(installPath, "embedded/.installed_by_pkg.txt")); err != nil {
		if os.IsNotExist(err) {
			return nil // No-op
		}
		return err
	}

	fmt.Println("Removing integrations installed with the 'agent integration' command")

	// Use an in-memory map to store all integration paths
	allIntegrations, err := getAllIntegrations(installPath)
	if err != nil {
		return err
	}

	// Read the list of installed files
	installedByPkg, err := os.ReadFile(filepath.Join(installPath, "embedded", ".installed_by_pkg.txt"))
	if err != nil {
		return err
	}

	// Create a set of paths installed by the package
	installedByPkgSet := make(map[string]struct{})
	for _, line := range strings.Split(string(installedByPkg), "\n") {
		if line != "" {
			// Make sure the path is absolute so we can compare apples to apples
			if !filepath.IsAbs(line) && !strings.HasPrefix(line, "#") {
				line = filepath.Join(installPath, line)
			}
			installedByPkgSet[line] = struct{}{}
		}
	}

	// Remove paths that are in allIntegrations but not in installedByPkgSet
	for _, path := range allIntegrations {
		if _, exists := installedByPkgSet[path]; !exists {
			// Remove if it was not installed by the package.
			if err := os.RemoveAll(path); err != nil {
				return err
			}
		}
	}

	return nil
}

// RemoveCompiledFiles removes compiled Python files (.pyc, .pyo) and __pycache__ directories
func RemoveCompiledFiles(installPath string) error {
	// Remove files in in "{installPath}/embedded/.py_compiled_files.txt"
	_, err := os.Stat(filepath.Join(installPath, "embedded/.py_compiled_files.txt"))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to check if compiled files list exists: %w", err)
	}
	if !os.IsNotExist(err) {
		compiledFiles, err := os.ReadFile(filepath.Join(installPath, "embedded/.py_compiled_files.txt"))
		if err != nil {
			return fmt.Errorf("failed to read compiled files list: %w", err)
		}
		for _, file := range strings.Split(string(compiledFiles), "\n") {
			if strings.HasPrefix(file, installPath) {
				if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("failed to remove compiled file %s: %w", file, err)
				}
			}
		}
	}
	// Remove files in {installPath}/bin/agent/dist
	err = filepath.Walk(filepath.Join(installPath, "bin", "agent", "dist"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if !os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() && info.Name() == "__pycache__" {
			if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		} else if strings.HasSuffix(info.Name(), ".pyc") || strings.HasSuffix(info.Name(), ".pyo") {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to remove compiled files: %w", err)
	}
	// Remove files in {installPath}/python-scripts
	err = filepath.Walk(filepath.Join(installPath, "python-scripts"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if !os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() && info.Name() == "__pycache__" {
			if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		} else if strings.HasSuffix(info.Name(), ".pyc") || strings.HasSuffix(info.Name(), ".pyo") {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to remove compiled files: %w", err)
	}
	return nil
}
