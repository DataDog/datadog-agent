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

const (
	baselineFileName = ".post_python_installed_packages.txt"
	diffFileName     = ".diff_python_installed_packages.txt"
)

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

// integrationStoragePath returns the directory that holds the integration
// save/restore files for installPath. OCI packages keep them in the persistent
// RunPath (the install dir is immutable and versioned); deb/rpm keep them in the
// install dir itself (== /opt/datadog-agent).
func integrationStoragePath(installPath string) string {
	if strings.HasPrefix(installPath, paths.PackagesPath) {
		return paths.RunPath
	}
	return installPath
}

// SaveCustomIntegrations saves custom integrations from the previous installation
// Today it calls pre.py to persist the custom integrations; though we should probably
// port this to Go in the future.
func SaveCustomIntegrations(ctx context.Context, installPath string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "save_custom_integrations")
	defer func() {
		span.Finish(err)
	}()
	storagePath := integrationStoragePath(installPath)
	if storagePath == paths.RunPath {
		if err := migrateLegacyOCIFile(paths.RootTmpDir, storagePath, baselineFileName); err != nil {
			return err
		}
	}
	if err := executePythonScript(ctx, installPath, "pre.py", installPath, storagePath); err != nil {
		return err
	}
	if storagePath == paths.RunPath {
		// An OCI package released before the move to RunPath restores the diff from
		// RootTmpDir, so mirror it there for such experiments.
		if err := mirrorOCIFile(storagePath, paths.RootTmpDir, diffFileName); err != nil {
			return err
		}
	}
	return nil
}

// migrateLegacyOCIFile seeds storageDir with a save/restore file an OCI package before
// this change wrote to the legacy RootTmpDir (the storage location used since 7.67.0,
// #36084), so an upgrade to a RunPath-based version still finds it. It copies the legacy
// file when the destination is missing, older than the legacy copy, or shares the legacy
// mtime but differs in content; a strictly newer destination is left untouched. No-op if
// the legacy file does not exist.
func migrateLegacyOCIFile(legacyDir, storageDir, fileName string) error {
	legacyPath := filepath.Join(legacyDir, fileName)
	storagePath := filepath.Join(storageDir, fileName)
	legacyInfo, err := os.Stat(legacyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to stat %s: %w", legacyPath, err)
	}
	storageInfo, err := os.Stat(storagePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to stat %s: %w", storagePath, err)
	}
	if err == nil {
		// A strictly newer destination is authoritative, so keep it. Equal mtimes are
		// ambiguous (e.g. both stamped within the same operation), so fall back to a
		// content comparison and only refresh when the bytes actually differ.
		if legacyInfo.ModTime().Before(storageInfo.ModTime()) {
			return nil
		}
		if legacyInfo.ModTime().Equal(storageInfo.ModTime()) {
			same, err := sameFileContents(legacyPath, storagePath)
			if err != nil {
				return err
			}
			if same {
				return nil
			}
		}
	}
	return copyOCIFile(legacyDir, storageDir, fileName)
}

// sameFileContents reports whether the files at the two paths have identical contents.
func sameFileContents(a, b string) (bool, error) {
	dataA, err := os.ReadFile(a)
	if err != nil {
		return false, fmt.Errorf("failed to read %s: %w", a, err)
	}
	dataB, err := os.ReadFile(b)
	if err != nil {
		return false, fmt.Errorf("failed to read %s: %w", b, err)
	}
	return string(dataA) == string(dataB), nil
}

// copyOCIFile copies fileName from srcDir to dstDir, overwriting any existing
// dstDir copy and creating dstDir if needed. No-op if the source file does not
// exist.
func copyOCIFile(srcDir, dstDir, fileName string) error {
	srcPath := filepath.Join(srcDir, fileName)
	dstPath := filepath.Join(dstDir, fileName)
	data, err := os.ReadFile(srcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read %s: %w", srcPath, err)
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("failed to create %s: %w", dstDir, err)
	}
	if err := os.WriteFile(dstPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write %s: %w", dstPath, err)
	}
	return nil
}

// mirrorOCIFile copies fileName into dstDir only when dstDir already exists, so a
// reaped RootTmpDir is not recreated (which would break experiment temp dirs).
func mirrorOCIFile(srcDir, dstDir, fileName string) error {
	if _, err := os.Stat(dstDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to stat %s: %w", dstDir, err)
	}
	return copyOCIFile(srcDir, dstDir, fileName)
}

// RestoreCustomIntegrations restores custom integrations from the previous installation
// Today it calls post.py to persist the custom integrations; though we should probably
// port this to Go in the future.
func RestoreCustomIntegrations(ctx context.Context, installPath string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "restore_custom_integrations")
	defer func() {
		span.Finish(err)
	}()

	storagePath := integrationStoragePath(installPath)

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

// RemoveCompiledFiles removes compiled Python files (__pycache__ folders)
func RemoveCompiledFiles(installPath string) error {
	// Remove files in {installPath}/embedded
	err := filepath.WalkDir(filepath.Join(installPath, "embedded"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() && d.Name() == "__pycache__" {
			if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
				return err
			}
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to remove compiled files: %w", err)
	}
	// Remove files in {installPath}/bin/agent/dist
	err = filepath.WalkDir(filepath.Join(installPath, "bin", "agent", "dist"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() && d.Name() == "__pycache__" {
			if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
				return err
			}
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to remove compiled files: %w", err)
	}
	// Remove files in {installPath}/python-scripts
	err = filepath.WalkDir(filepath.Join(installPath, "python-scripts"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() && d.Name() == "__pycache__" {
			if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
				return err
			}
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to remove compiled files: %w", err)
	}
	return nil
}
