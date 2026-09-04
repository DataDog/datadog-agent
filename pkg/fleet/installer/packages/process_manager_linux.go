// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SetProcessManagerEnabled flips the effective process manager for the agent's supervised
// components between dd-procmgrd and native systemd, and reconciles the running services so the
// change takes effect immediately. It is a no-op if the desired state already matches the current
// one.
func SetProcessManagerEnabled(ctx context.Context, enabled bool) error {
	if env.FromEnv().ProcessManagerEnabled == enabled {
		return nil
	}
	installRoot := filepath.Join(paths.PackagesPath, agentPackage, "stable")
	switch service.GetServiceManagerType(installRoot) {
	case service.SystemdType, service.ProcmgrType:
	default:
		return errors.New("switching the process manager is only supported under systemd")
	}

	// Switching requires everything to be stable: an in-progress experiment means the stable and
	// experiment unit sets can diverge on which process manager they expect, and tearing stable
	// down here would leave the experiment referencing units that no longer exist.
	state, err := repository.NewRepositories(paths.PackagesPath, AsyncPreRemoveHooks).GetState(agentPackage)
	if err != nil {
		return fmt.Errorf("failed to get agent package state: %w", err)
	}
	if state.HasExperiment() {
		return errors.New("cannot switch the process manager while an experiment is in progress")
	}

	// PackageType only affects checkPlatformSupport's upstart/sysvinit gating, and the switch
	// above already restricts this function to systemd/procmgr — so it plays no role here and
	// this works identically for OCI and deb/rpm installs alike.
	hookCtx := HookContext{Context: ctx, PackagePath: installRoot, PackageType: PackageTypeOCI}

	if err := agentService.StopStable(hookCtx); err != nil {
		log.Warnf("failed to stop stable units: %v", err)
	}
	if err := agentService.DisableStable(hookCtx); err != nil {
		log.Warnf("failed to disable stable units: %v", err)
	}
	if err := agentService.RemoveStable(hookCtx); err != nil {
		log.Warnf("failed to remove stable units: %v", err)
	}

	// Flip this process's own view of the desired state so that GetServiceManagerType(installRoot)
	// resolves to the newly-selected type for the remainder of this call. The old unit set was
	// already torn down above, so a failure here must not abort: doing so would leave the host
	// with no service at all. Instead keep going — GetServiceManagerType will resolve back to
	// whichever type it saw before (the switch didn't take effect) — and report the failure once
	// the service is back up.
	var setEnvErr error
	if err := os.Setenv(env.EnvProcessManagerEnabled, strconv.FormatBool(enabled)); err != nil {
		setEnvErr = fmt.Errorf("failed to set process manager state: %w", err)
		log.Warnf("%v", setEnvErr)
	}

	if err := agentService.WriteProcesses(installRoot); err != nil {
		return errors.Join(setEnvErr, fmt.Errorf("failed to write processes: %w", err))
	}
	if err := agentService.WriteStable(hookCtx); err != nil {
		return errors.Join(setEnvErr, fmt.Errorf("failed to write stable units: %w", err))
	}
	if err := agentService.EnableStable(hookCtx); err != nil {
		return errors.Join(setEnvErr, fmt.Errorf("failed to enable stable units: %w", err))
	}
	if err := agentService.RestartStable(hookCtx); err != nil {
		return errors.Join(setEnvErr, err)
	}
	return setEnvErr
}
