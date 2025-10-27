// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin || linux

package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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
	stableDeploymentID, err := os.ReadFile(stablePath)
	if err != nil && !os.IsNotExist(err) {
		return State{}, err
	}
	experimentDeploymentID, err := os.ReadFile(experimentPath)
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
	err := os.RemoveAll(d.ExperimentPath)
	if err != nil {
		return err
	}
	err = copyDirectory(ctx, d.StablePath, d.ExperimentPath)
	if err != nil {
		return err
	}

	operations.FileOperations = append(buildOperationsFromLegacyInstaller(d.StablePath), operations.FileOperations...)

	err = operations.Apply(d.ExperimentPath)
	if err != nil {
		return err
	}
	err = os.WriteFile(filepath.Join(d.ExperimentPath, deploymentIDFile), []byte(operations.DeploymentID), 0640)
	if err != nil {
		return err
	}
	return nil
}

// PromoteExperiment promotes the experiment to the stable.
func (d *Directories) PromoteExperiment(_ context.Context) error {
	_, err := os.Stat(d.ExperimentPath)
	if err != nil {
		return err
	}
	err = replaceConfigDirectory(d.StablePath, d.ExperimentPath)
	if err != nil {
		return err
	}
	return nil
}

// RemoveExperiment removes the experiment from the directories.
func (d *Directories) RemoveExperiment(_ context.Context) error {
	err := os.RemoveAll(d.ExperimentPath)
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
	cmd := telemetry.CommandContext(ctx, "cp", "-a", sourcePath, targetPath)
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
	backupPath := filepath.Clean(oldDir) + ".bak"
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
	err = os.RemoveAll(backupPath)
	if err != nil {
		return fmt.Errorf("could not remove old directory: %w", err)
	}
	return nil
}
