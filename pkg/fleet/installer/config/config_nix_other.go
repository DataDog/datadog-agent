// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && !darwin

package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/file"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// The Unix configuration directories (config_nix.go) differ by platform only through the
// functions and constants below. This file holds the Linux side, and every other Unix but macOS.

const (
	// cpPath is the cp the configuration is copied with, looked up in PATH.
	cpPath = "cp"
	// migrateLegacyLayout reports whether an experiment folds in the legacy installer's managed
	// configuration files (buildOperationsFromLegacyInstaller).
	migrateLegacyLayout = true
)

// readDeploymentID reads a deployment ID file.
func readDeploymentID(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// experimentSuperseded reports whether the experiment directory is the stable one a promote
// replaced. The promote here renames the directories, which leaves nothing behind to mistake for
// an experiment.
func experimentSuperseded(_ string) bool {
	return false
}

// exchangeDirectories is not supported here: PromoteExperiment renames the directories instead
// (replaceConfigDirectory).
func exchangeDirectories(_, _ string) error {
	return errors.ErrUnsupported
}

// stageExperiment replaces the experiment directory with a copy of the stable one, then lets fill
// write the experiment into it.
func (d *Directories) stageExperiment(ctx context.Context, fill func(dir string) error) error {
	err := paths.RemoveAll(ctx, d.ExperimentPath)
	if err != nil {
		return err
	}
	err = copyDirectory(ctx, d.StablePath, d.ExperimentPath)
	if err != nil {
		return err
	}
	return fill(d.ExperimentPath)
}

// setFileOwnershipAndPermissions sets the ownership and permissions for a file based on its configFileSpec.
// If the user doesn't exist (e.g., in tests) or if we don't have permission
// to change ownership, the function logs a warning and continues without failing.
func setFileOwnershipAndPermissions(ctx context.Context, root *os.Root, path string, spec *configFileSpec) error {
	// Set file permissions
	if spec.mode != 0 {
		if err := root.Chmod(path, spec.mode); err != nil {
			return fmt.Errorf("error setting file permissions for %s: %w", path, err)
		}
	}

	// Set file ownership
	err := file.Chown(ctx, filepath.Join(root.Name(), path), spec.owner, spec.group)
	if err != nil {
		log.Warnf("error setting file ownership for %s: %v", path, err)
	}

	return nil
}
