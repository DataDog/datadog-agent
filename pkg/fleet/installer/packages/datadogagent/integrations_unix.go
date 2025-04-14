// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package datadogagent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

// saveCustomIntegrations saves custom integrations from the previous installation
// Today it calls pre.py to persist the custom integrations; though we should probably
// port this to Go in the future.
//
// Note: in the OCI installation this fails as the file where integrations are saved
// is hardcoded to be in the same directory as the agent. This will be fixed in a future PR.
func saveCustomIntegrations(ctx context.Context, installPath string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "save_custom_integrations")
	defer func() {
		span.Finish(err)
	}()

	if _, err := os.Stat(filepath.Join(installPath, "embedded/bin/python")); err == nil {
		cmd := exec.CommandContext(ctx, filepath.Join(installPath, "embedded/bin/python"), filepath.Join(installPath, "python-scripts/pre.py"), installPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to run integration persistence in pre.py: %w", err)
		}
	}
	return nil
}

// restoreCustomIntegrations restores custom integrations from the previous installation
// Today it calls post.py to persist the custom integrations; though we should probably
// port this to Go in the future.
//
// Note: in the OCI installation this fails as the file where integrations are saved
// is hardcoded to be in the same directory as the agent. This will be fixed in a future PR.
func restoreCustomIntegrations(ctx context.Context, installPath string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "restore_custom_integrations")
	defer func() {
		span.Finish(err)
	}()

	if _, err := os.Stat(filepath.Join(installPath, "embedded/bin/python")); err == nil {
		cmd := exec.CommandContext(ctx, filepath.Join(installPath, "embedded/bin/python"), filepath.Join(installPath, "python-scripts/post.py"), installPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to run integration persistence in post.py: %w", err)
		}
	}
	return nil
}

// removeCustomIntegrations removes custom integrations that are not installed by the package
//
// Since 6.18.0, a file containing all integrations files which have been installed by
// the package is available. We use it to remove only the datadog-related check files which
// have *NOT* been installed by the package (eg: installed using the `integration` command).
func removeCustomIntegrations(ctx context.Context, installPath string) (err error) {
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
	allIntegrations, err := filepath.Glob(installPath + "/embedded/lib/python*/site-packages/datadog_*")
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
			installedByPkgSet[line] = struct{}{}
		}
	}

	// Remove paths that are in allIntegrations but not in installedByPkgSet
	for _, path := range allIntegrations {
		if _, exists := installedByPkgSet[path]; !exists {
			// Remove the directory if it was not installed by the package.
			if err := os.RemoveAll(path); err != nil {
				return err
			}
		}
	}

	return nil
}
