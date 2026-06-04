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
	"path/filepath"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

var (
	datadogInstalledIntegrationsPattern = regexp.MustCompile(`embedded/lib/python[^/]+/site-packages/datadog_.*`)
)

const baselineFileName = ".post_python_installed_packages.txt"
const diffFileName = ".diff_python_installed_packages.txt"

// executePythonScript executes a Python script with the given arguments
func executePythonScript(ctx context.Context, installPath, scriptName string, args ...string) error {
	pythonPath := filepath.Join(installPath, "embedded/bin/python")
	scriptPath := filepath.Join(installPath, "python-scripts", scriptName)

	if _, err := os.Stat(pythonPath); err != nil {
		return fmt.Errorf("python not found at %s: %w", pythonPath, err)
	}
	if err := os.RemoveAll(filepath.Join(installPath, "python-scripts/__pycache__")); err != nil {
		return fmt.Errorf("failed to remove __pycache__ at %s: %w", filepath.Join(installPath, "python-scripts/__pycache__"), err)
	}

	pythonCmd := append([]string{"-B", scriptPath}, args...)
	cmd := telemetry.CommandContext(ctx, pythonPath, pythonCmd...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run %s: %w", scriptName, err)
	}

	return nil
}

// SaveCustomIntegrations saves custom integrations from the previous installation
// Today it calls pre.py to persist the custom integrations; though we should probably
// port this to Go in the future.
func SaveCustomIntegrations(ctx context.Context, installPath string, storagePath string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "save_custom_integrations")
	defer func() {
		span.Finish(err)
	}()
	if storagePath == paths.RunPath {
		if err := migrateLegacyOCIFile(paths.RootTmpDir, storagePath, baselineFileName); err != nil {
			return err
		}
	}
	return executePythonScript(ctx, installPath, "pre.py", installPath, storagePath)
}

// migrateLegacyOCIFile seeds storageDir with a save/restore file an older OCI
// package wrote to the legacy RootTmpDir, so the first upgrade to a RunPath-based
// version still finds it. No-op if the destination exists or the legacy file does not.
func migrateLegacyOCIFile(legacyDir, storageDir, fileName string) error {
	dst := filepath.Join(storageDir, fileName)
	if _, err := os.Stat(dst); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to stat %s: %w", dst, err)
	}
	data, err := os.ReadFile(filepath.Join(legacyDir, fileName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read legacy %s: %w", fileName, err)
	}
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		return fmt.Errorf("failed to create storage dir %s: %w", storageDir, err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return fmt.Errorf("failed to write %s to %s: %w", fileName, dst, err)
	}
	return nil
}

// RestoreCustomIntegrations restores custom integrations from the previous installation
// Today it calls post.py to persist the custom integrations; though we should probably
// port this to Go in the future.
func RestoreCustomIntegrations(ctx context.Context, installPath string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "restore_custom_integrations")
	defer func() {
		span.Finish(err)
	}()

	// For OCI packages, the baseline must persist across upgrades (days-weeks),
	// so it lives in the persistent RunPath rather than the 24h-reaped RootTmpDir.
	// For deb/rpm (installPath == /opt/datadog-agent) it stays in the install dir.
	storagePath := installPath
	if strings.HasPrefix(installPath, paths.PackagesPath) {
		storagePath = paths.RunPath
	}

	// An older package's pre.py wrote the diff to RootTmpDir; this restore reads RunPath.
	// Seed it so the first upgrade to this version restores instead of dropping integrations.
	if storagePath == paths.RunPath {
		if err := migrateLegacyOCIFile(paths.RootTmpDir, storagePath, diffFileName); err != nil {
			return err
		}
	}

	return executePythonScript(ctx, installPath, "post.py", installPath, storagePath)
}

// getAllIntegrations retrieves all integration paths installed by the package
// It walks through the installPath and collects paths that match the './embedded/lib/python*/site-packages/datadog_*' pattern.
func getAllIntegrations(installPath string) ([]string, error) {
	allIntegrations := make([]string, 0)
	err := filepath.WalkDir(installPath, func(path string, _ os.DirEntry, err error) error {
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
	for line := range strings.SplitSeq(string(installedByPkg), "\n") {
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
		for file := range strings.SplitSeq(string(compiledFiles), "\n") {
			if strings.HasPrefix(file, installPath) {
				if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("failed to remove compiled file %s: %w", file, err)
				}
			}
		}
	}
	// Remove files in {installPath}/bin/agent/dist
	err = filepath.WalkDir(filepath.Join(installPath, "bin", "agent", "dist"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if !os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() && d.Name() == "__pycache__" {
			if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		} else if strings.HasSuffix(d.Name(), ".pyc") || strings.HasSuffix(d.Name(), ".pyo") {
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
	err = filepath.WalkDir(filepath.Join(installPath, "python-scripts"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if !os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() && d.Name() == "__pycache__" {
			if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		} else if strings.HasSuffix(d.Name(), ".pyc") || strings.HasSuffix(d.Name(), ".pyo") {
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
