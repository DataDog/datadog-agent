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
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// processManagerEnvVar must match envProcessManagerEnabled in pkg/fleet/installer/env.
const processManagerEnvVar = "DD_PROCESS_MANAGER_ENABLED"

// SetProcessManagerEnabled flips the effective process manager for the agent's supervised
// components between dd-procmgrd and native systemd, and reconciles the running services so the
// change takes effect immediately. It is a no-op if the desired state already matches the current
// one.
//
// The reconciliation reuses the existing datadogAgentService abstraction
// (StopStable/DisableStable/RemoveStable/WriteProcesses/WriteStable/EnableStable/RestartStable),
// which already dispatches on service.GetServiceManagerType() on every call — the same
// mechanism a normal install/update/experiment hook uses to pick systemd's or procmgr's unit
// set. GetServiceManagerType is not memoized for the procmgr decision (only the init-system
// probe is, see pkg/fleet/installer/packages/service), so tearing the currently-active type's
// units down BEFORE flipping the in-process env var, then writing the newly-selected type's units
// AFTER, moves every managed unit between the two modes without hand-rolling unit-specific
// stop/start logic here — exactly mirroring what preInstallDatadogAgent/postInstallDatadogAgent
// already do for a normal reinstall. The freshly-written installer unit itself carries the
// correct DD_PROCESS_MANAGER_ENABLED value baked in (see
// packages/embedded/tmpl/datadog-agent-installer.service.tmpl), so the daemon picks it up on its
// own once RestartStable restarts it under systemd — no persistence beyond this process's
// lifetime is needed.
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

	// Flipping the installer's own process manager is scoped to OCI installs (the fleet-managed
	// installation method this command and the daemon are built for); deb/rpm installs manage
	// their own systemd units directly via the package manager.
	hookCtx := HookContext{Context: ctx, PackagePath: installRoot, PackageType: PackageTypeOCI}

	// Tear down the currently-active unit set while GetServiceManagerType still resolves to it.
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
	// resolves to the newly-selected type for the remainder of this call.
	if err := os.Setenv(processManagerEnvVar, strconv.FormatBool(enabled)); err != nil {
		return fmt.Errorf("failed to set process manager state: %w", err)
	}

	if err := agentService.WriteProcesses(installRoot); err != nil {
		return fmt.Errorf("failed to write processes: %w", err)
	}
	if err := agentService.WriteStable(hookCtx); err != nil {
		return fmt.Errorf("failed to write stable units: %w", err)
	}
	if err := agentService.EnableStable(hookCtx); err != nil {
		return fmt.Errorf("failed to enable stable units: %w", err)
	}
	return agentService.RestartStable(hookCtx)
}
