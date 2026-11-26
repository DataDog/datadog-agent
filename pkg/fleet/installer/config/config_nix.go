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

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/symlink"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

const (
	deploymentIDFile = ".deployment-id"
)

// GetState returns the state of the directories.
func (d *Directories) GetState() (State, error) {
	stablePath := filepath.Join(d.StablePath, deploymentIDFile)
	experimentPath := filepath.Join(d.ExperimentPath, deploymentIDFile)
	stableDeploymentID, err := readDeploymentID(stablePath)
	if err != nil && !os.IsNotExist(err) {
		return State{}, err
	}
	experimentDeploymentID, err := readDeploymentID(experimentPath)
	if err != nil && !os.IsNotExist(err) {
		return State{}, err
	}
	stableExists := len(stableDeploymentID) > 0
	experimentExists := len(experimentDeploymentID) > 0
	// If experiment is symlinked to stable, it means the experiment is not installed.
	if stableExists && experimentExists && isSameFile(stablePath, experimentPath) {
		experimentDeploymentID = nil
	}
	return State{
		StableDeploymentID:     string(stableDeploymentID),
		ExperimentDeploymentID: string(experimentDeploymentID),
	}, nil
}

// WriteExperiment writes the experiment to the directories.
func (d *Directories) WriteExperiment(ctx context.Context, operations Operations) error {
	return d.stageExperiment(ctx, func(dir string) error {
		if migrateLegacyLayout {
			operations.FileOperations = append(buildOperationsFromLegacyInstaller(d.StablePath), operations.FileOperations...)
		}
		err := operations.Apply(ctx, dir)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, deploymentIDFile), []byte(operations.DeploymentID), 0640)
	})
}

// PromoteExperiment promotes the experiment to the stable.
func (d *Directories) PromoteExperiment(ctx context.Context) error {
	_, err := os.Stat(d.ExperimentPath)
	if err != nil {
		return err
	}
	err = exchangeDirectories(d.StablePath, d.ExperimentPath)
	if errors.Is(err, errors.ErrUnsupported) {
		return replaceConfigDirectory(d.StablePath, d.ExperimentPath)
	}
	if err != nil {
		return err
	}
	// The exchange leaves the previous stable directory at the experiment path.
	return d.RemoveExperiment(ctx)
}

// RemoveExperiment removes the experiment from the directories.
func (d *Directories) RemoveExperiment(ctx context.Context) error {
	err := paths.RemoveAll(ctx, d.ExperimentPath)
	if err != nil {
		return err
	}
	err = symlink.Set(d.ExperimentPath, d.StablePath)
	if err != nil {
		return err
	}
	return nil
}

// copyDirectory copies a directory from source to target.
// It preserves the directory structure and file permissions.
func copyDirectory(ctx context.Context, sourcePath, targetPath string) error {
	cmd := telemetry.CommandContext(ctx, cpPath, "-a", sourcePath, targetPath)
	return cmd.Run()
}

func isSameFile(file1, file2 string) bool {
	stat1, err := os.Stat(file1)
	if err != nil {
		return false
	}
	stat2, err := os.Stat(file2)
	if err != nil {
		return false
	}
	return os.SameFile(stat1, stat2)
}

// replaceConfigDirectory replaces the contents of two directories.
func replaceConfigDirectory(oldDir, newDir string) (err error) {
	backupDir, err := os.MkdirTemp(filepath.Dir(oldDir), "datadog-backup")
	if err != nil {
		return fmt.Errorf("could not create backup directory: %w", err)
	}
	defer os.RemoveAll(backupDir)
	backupPath := filepath.Join(backupDir, filepath.Base(oldDir))
	err = os.Rename(oldDir, backupPath)
	if err != nil {
		return fmt.Errorf("could not rename old directory: %w", err)
	}
	defer func() {
		if err != nil {
			rollbackErr := os.Rename(backupPath, oldDir)
			if rollbackErr != nil {
				err = fmt.Errorf("%w, rollback error: %w", err, rollbackErr)
			}
		}
	}()
	err = os.Rename(newDir, oldDir)
	if err != nil {
		return fmt.Errorf("could not rename new directory: %w", err)
	}
	return nil
}
