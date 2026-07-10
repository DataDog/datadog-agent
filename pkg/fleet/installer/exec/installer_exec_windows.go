// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package exec provides an implementation of the Installer interface that uses the installer binary.
package exec

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/config"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/extensions"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

const packageDatadogAgent = "datadog-agent"

func (i *InstallerExec) newInstallerCmdPlatform(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
	return cmd
}

// getStates retrieves the state of all packages & their configuration from disk.
// On Windows there is no privilege boundary between the daemon and the installer
// binary, so we read the package & config states in-process instead of spawning
// a subprocess (which is the main source of OOM errors on Windows).
func (i *InstallerExec) getStates(ctx context.Context) (_ *repository.ConfigAndPackageStates, err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "installer.get-states")
	defer func() { span.Finish(err) }()

	repos := repository.NewRepositories(paths.PackagesPath, nil)
	packageStates, err := repos.GetStates()
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("error getting package states from disk: %w", err)
	}
	if packageStates == nil {
		packageStates = make(map[string]repository.State)
	}

	configDirs := &config.Directories{
		StablePath:     paths.AgentConfigDir,
		ExperimentPath: paths.AgentConfigDirExp,
	}
	configState, err := configDirs.GetState()
	if err != nil {
		return nil, fmt.Errorf("error getting config state from disk: %w", err)
	}
	stableDeploymentID := configState.StableDeploymentID
	if stableDeploymentID == "" {
		stableDeploymentID = "empty"
	}
	configStates := map[string]repository.State{
		packageDatadogAgent: {
			Stable:     stableDeploymentID,
			Experiment: configState.ExperimentDeploymentID,
		},
	}

	result := &repository.ConfigAndPackageStates{
		ConfigStates:  configStates,
		PackageStates: make(map[string]repository.PackageState),
	}
	for pkg, s := range packageStates {
		stableExtensions, err := extensions.InstalledExtensions(pkg, false)
		if err != nil {
			return nil, fmt.Errorf("error getting installed stable extensions for %s: %w", pkg, err)
		}
		experimentExtensions, err := extensions.InstalledExtensions(pkg, true)
		if err != nil {
			return nil, fmt.Errorf("error getting installed experiment extensions for %s: %w", pkg, err)
		}
		result.PackageStates[pkg] = repository.PackageState{
			Stable: repository.VersionState{
				Version:    s.Stable,
				Extensions: stableExtensions,
			},
			Experiment: repository.VersionState{
				Version:    s.Experiment,
				Extensions: experimentExtensions,
			},
		}
	}

	return result, nil
}
