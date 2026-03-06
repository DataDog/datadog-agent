// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"

	"golang.org/x/sys/windows"
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
		return State{}, fmt.Errorf("error reading stable deployment ID file: %w", err)
	}
	experimentDeploymentID, err := os.ReadFile(experimentPath)
	if err != nil && !os.IsNotExist(err) {
		return State{}, fmt.Errorf("error reading experiment deployment ID file: %w", err)
	}
	return State{
		StableDeploymentID:     string(stableDeploymentID),
		ExperimentDeploymentID: string(experimentDeploymentID),
	}, nil
}

// WriteExperiment writes the experiment to the directories.
func (d *Directories) WriteExperiment(ctx context.Context, operations Operations) error {
	state, err := d.GetState()
	if err != nil {
		return fmt.Errorf("error getting state: %w", err)
	}
	if state.ExperimentDeploymentID != "" {
		return errors.New("there is already an experiment in progress")
	}
	// Clear and recreate the experiment/backup directory
	err = os.RemoveAll(d.ExperimentPath)
	if err != nil {
		return fmt.Errorf("error removing experiment directory: %w", err)
	}
	err = secureCreateTargetDirectoryWithSourcePermissions(d.StablePath, d.ExperimentPath)
	if err != nil {
		return fmt.Errorf("error creating target directory: %w", err)
	}
	err = backupOrRestoreDirectory(ctx, d.StablePath, d.ExperimentPath)
	if err != nil {
		return fmt.Errorf("error writing deployment ID file: %w", err)
	}
	operations.FileOperations = append(buildOperationsFromLegacyInstaller(d.StablePath), operations.FileOperations...)
	err = operations.Apply(ctx, d.StablePath)
	if err != nil {
		return fmt.Errorf("error applying operations: %w", err)
	}
	err = os.WriteFile(filepath.Join(d.ExperimentPath, deploymentIDFile), []byte(operations.DeploymentID), 0640)
	if err != nil {
		return fmt.Errorf("error writing deployment ID file: %w", err)
	}
	return nil
}

// PromoteExperiment promotes the experiment to the stable.
func (d *Directories) PromoteExperiment(_ context.Context) error {
	_, err := os.Stat(d.ExperimentPath)
	if err != nil {
		return fmt.Errorf("error checking for experiment directory: %w", err)
	}
	_, err = os.Stat(filepath.Join(d.ExperimentPath, deploymentIDFile))
	if err != nil {
		return fmt.Errorf("error checking for deployment ID file: %w", err)
	}
	err = os.Rename(filepath.Join(d.ExperimentPath, deploymentIDFile), filepath.Join(d.StablePath, deploymentIDFile))
	if err != nil {
		return fmt.Errorf("error renaming deployment ID file: %w", err)
	}
	err = os.RemoveAll(d.ExperimentPath)
	if err != nil {
		return fmt.Errorf("error removing experiment directory: %w", err)
	}
	return nil
}

// RemoveExperiment removes the experiment from the directories.
func (d *Directories) RemoveExperiment(ctx context.Context) error {
	_, err := os.Stat(d.ExperimentPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error checking for experiment directory: %w", err)
	}
	if os.IsNotExist(err) {
		return nil
	}
	// Skip copying deployment ID during rollback - we want to preserve stable's deployment ID
	err = backupOrRestoreDirectory(ctx, d.ExperimentPath, d.StablePath)
	if err != nil {
		return fmt.Errorf("error backing up stable directory: %w", err)
	}
	err = os.RemoveAll(d.ExperimentPath)
	if err != nil {
		return fmt.Errorf("error removing experiment directory: %w", err)
	}
	return nil
}

// backupOrRestoreDirectory copies YAML files from source to target.
// It preserves the directory structure and file permissions.
// If copyDeploymentID is true, also copies the .deployment-id file.
func backupOrRestoreDirectory(ctx context.Context, sourcePath, targetPath string) error {
	_, err := os.Stat(sourcePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error checking if source directory exists: %w", err)
	}
	if os.IsNotExist(err) {
		return nil
	}
	// target path must already exist, caller must ensure it has the correct permissions.
	// We must not allow robocopy to create the target directory, as it may not safely create the directory,
	// see paths.SecureCreateDirectory for more details.
	// This function is reused for the restore operation, too, so this is also a safety to ensure
	// we don't modify the original directory.
	if _, err := os.Stat(targetPath); err != nil {
		return fmt.Errorf("failed to open target directory: %w", err)
	}

	cmd := telemetry.CommandContext(
		ctx,
		"robocopy",
		"/MIR",
		"/SL",
		sourcePath,
		targetPath,
		"*.yaml",
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return fmt.Errorf("error executing robocopy: %w", err)
	}
	if exitErr != nil && exitErr.ExitCode() >= 8 {
		return fmt.Errorf("error executing robocopy: %w\n%s\n%s", err, stdout.String(), stderr.String())
	}
	return nil
}

// secureCreateTargetDirectoryWithSourcePermissions creates targetPath with the same permissions as srcPath.
//
// See paths.SecureCreateDirectory for more details.
func secureCreateTargetDirectoryWithSourcePermissions(sourcePath, targetPath string) error {
	sd, err := windows.GetNamedSecurityInfo(sourcePath, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.GROUP_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("error getting security info for source directory: %w", err)
	}
	sddl := sd.String()
	return paths.SecureCreateDirectory(targetPath, sddl)
}

// setFileOwnershipAndPermissions is a no-op on Windows as file ownership and permissions
// are handled differently through ACLs, not POSIX ownership and modes.
func setFileOwnershipAndPermissions(_ context.Context, _ *os.Root, _ string, _ *configFileSpec) error {
	return nil
}
