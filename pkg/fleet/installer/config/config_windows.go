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
		return fmt.Errorf("error restoring stable directory: %w", err)
	}
	// robocopy does not carry ACLs, so re-grant Everyone read on application_monitoring.yaml
	// (the only world-readable config file) after restoring the stable directory.
	if err := grantApplicationMonitoringReadAccess(d.StablePath); err != nil {
		return fmt.Errorf("error applying application_monitoring.yaml permissions: %w", err)
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

	// robocopy exit codes 1-7 indicate successful copies with various informational statuses.
	// Only codes >=8 indicate copy errors.
	// https://learn.microsoft.com/en-us/troubleshoot/windows-server/backup-and-storage/return-codes-used-robocopy-utility
	cmd := telemetry.CommandContext(
		ctx,
		"robocopy",
		"/MIR",
		"/SL",
		sourcePath,
		targetPath,
		"*.yaml",
	).WithExpectedExitCodes(1, 2, 3, 4, 5, 6, 7)
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

// setFileOwnershipAndPermissions sets ACLs for a config file based on its configFileSpec.
//
// Windows has no POSIX ownership; ACLs are inherited from C:\ProgramData\Datadog, which is
// restricted to Administrators and ddagentuser. For files Linux makes world-readable
// (mode 0644 — only application_monitoring.yaml), we grant Everyone read so non-admin
// identities (e.g. IIS App Pool) can read fleet config. Restricted files (mode 0640) keep the
// inherited admin/ddagentuser-only ACL — we only ever grant, never modify other files' ACLs.
func setFileOwnershipAndPermissions(_ context.Context, root *os.Root, path string, spec *configFileSpec) error {
	if spec.mode&0o004 == 0 {
		return nil
	}
	return paths.SetFileReadableByEveryone(filepath.Join(root.Name(), path))
}

// grantApplicationMonitoringReadAccess re-grants Everyone read on application_monitoring.yaml
// under stablePath, if it exists. application_monitoring.yaml is the only world-readable config
// file, and robocopy (used to restore the stable directory during a rollback) does not carry
// ACLs, so the ACE must be reapplied afterward.
func grantApplicationMonitoringReadAccess(stablePath string) error {
	appMonitoringPath := filepath.Join(stablePath, "application_monitoring.yaml")
	_, err := os.Stat(appMonitoringPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("error checking application_monitoring.yaml: %w", err)
	}
	return paths.SetFileReadableByEveryone(appMonitoringPath)
}
