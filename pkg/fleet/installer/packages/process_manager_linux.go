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
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/systemd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// processManagerEnvVar must match envProcessManagerEnabled in pkg/fleet/installer/env.
const processManagerEnvVar = "DD_PROCESS_MANAGER_ENABLED"

// installerServiceUnits are the installer daemon's own systemd units. Persisting
// DD_PROCESS_MANAGER_ENABLED into their environment is what lets the daemon (and any hook
// subprocess it spawns) know the last-chosen process manager across restarts, updates, and
// experiments, without an explicit override on every invocation.
var installerServiceUnits = []string{"datadog-agent-installer.service", "datadog-agent-installer-exp.service"}

// SetProcessManagerEnabled flips the effective process manager for the agent's supervised
// components between dd-procmgrd and native systemd, persisting the choice into the installer
// daemon's own service environment and reconciling the running services so the change takes
// effect immediately. It is a no-op if the desired state already matches the current one.
//
// The reconciliation reuses the existing datadogAgentService abstraction
// (StopStable/DisableStable/RemoveStable/WriteProcesses/WriteStable/EnableStable/RestartStable),
// which already dispatches on service.GetServiceManagerType() on every call — the same
// mechanism a normal install/update/experiment hook uses to pick systemd's or procmgr's unit
// set. GetServiceManagerType is not memoized for the procmgr decision (only the init-system
// probe is, see pkg/fleet/installer/packages/service), so tearing the currently-active type's
// units down BEFORE persisting the new value, then writing the newly-selected type's units
// AFTER, moves every managed unit between the two modes without hand-rolling unit-specific
// stop/start logic here — exactly mirroring what preInstallDatadogAgent/postInstallDatadogAgent
// already do for a normal reinstall.
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

	if err := persistProcessManagerEnabled(ctx, enabled); err != nil {
		return fmt.Errorf("failed to persist process manager state: %w", err)
	}

	// From here, GetServiceManagerType(installRoot) resolves to the newly-selected type.
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

// persistProcessManagerEnabled writes DD_PROCESS_MANAGER_ENABLED into the shared environment
// override file (read back by env.ProcessManagerEnabledFromEnv as the persisted fallback for
// every caller) and ensures the installer's own systemd units source it too, so the daemon's
// own process environment reflects the choice on its next start.
func persistProcessManagerEnabled(ctx context.Context, enabled bool) error {
	if err := upsertEnvFileValue(env.ProcessManagerEnvFilePath, processManagerEnvVar, strconv.FormatBool(enabled)); err != nil {
		return err
	}
	running, err := systemd.IsRunning()
	if err != nil {
		return fmt.Errorf("check systemd running: %w", err)
	}
	if !running {
		return nil
	}
	for _, unit := range installerServiceUnits {
		content := fmt.Sprintf("[Service]\nEnvironmentFile=-%s\n", env.ProcessManagerEnvFilePath)
		if err := systemd.WriteUnitOverride(ctx, unit, "process_manager", content); err != nil {
			return fmt.Errorf("failed to write process manager override for %s: %w", unit, err)
		}
	}
	return systemd.Reload(ctx)
}

// upsertEnvFileValue sets key=value in the key=value environment file at path, replacing any
// existing line for key and preserving every other line, creating the file/directory if needed.
func upsertEnvFileValue(path string, key string, value string) error {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read %s: %w", path, err)
	}

	var lines []string
	found := false
	if len(existing) > 0 {
		for _, line := range strings.Split(strings.TrimRight(string(existing), "\n"), "\n") {
			if strings.HasPrefix(line, key+"=") {
				lines = append(lines, key+"="+value)
				found = true
				continue
			}
			lines = append(lines, line)
		}
	}
	if !found {
		lines = append(lines, key+"="+value)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create directory for %s: %w", path, err)
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}
