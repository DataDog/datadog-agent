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
	"path/filepath"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/config"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/db"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/garbagecollect"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/apmlibrarydotnet"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

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
func (i *InstallerExec) getStates(ctx context.Context) (_ *repository.PackageStates, err error) {
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
		"datadog-agent": {
			Stable:     stableDeploymentID,
			Experiment: configState.ExperimentDeploymentID,
		},
	}

	return &repository.PackageStates{
		States:       packageStates,
		ConfigStates: configStates,
	}, nil
}

// GarbageCollect runs the garbage collector.
// On Windows there is no privilege boundary between the daemon and the installer
// binary, so we clean up unused packages and temporary files in-process instead
// of spawning a subprocess.
func (i *InstallerExec) GarbageCollect(ctx context.Context) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "installer.garbage-collect")
	defer func() { span.Finish(err) }()

	if err := paths.EnsureInstallerDirectories(); err != nil {
		return fmt.Errorf("could not ensure packages and config directory exists: %w", err)
	}

	// Hold the same packages database lock as the installer command while
	// deleting unused package directories.
	packagesDB, err := db.New(ctx, filepath.Join(paths.PackagesPath, "packages.db"), db.WithTimeout(5*time.Minute))
	if err != nil {
		return fmt.Errorf("could not create packages db: %w", err)
	}
	defer func() {
		if dbErr := packagesDB.Close(); dbErr != nil {
			dbErr = fmt.Errorf("failed to close packages database: %w", dbErr)
			err = errors.Join(err, dbErr)
		}
	}()

	repos := repository.NewRepositories(paths.PackagesPath, map[string]repository.PreRemoveHook{
		apmlibrarydotnet.PackageName: apmlibrarydotnet.AsyncPreRemoveHook,
	})
	return garbagecollect.Run(ctx, repos, paths.RootTmpDir)
}
