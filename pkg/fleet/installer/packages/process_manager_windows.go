// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"context"
	"fmt"
	"slices"

	"golang.org/x/sys/windows/registry"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/processmanager"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// processManagerEnvVar must match envProcessManagerEnabled in pkg/fleet/installer/env.
const processManagerEnvVar = "DD_PROCESS_MANAGER_ENABLED"

// parServiceName is PAR's native SCM service (cmd/agent/subcommands/run/dependent_services_windows.go),
// distinct from otelServiceName. It ships as part of the base Agent MSI, so it is always
// present to stop/start directly, unlike DDOT's dormant fallback which is only registered when
// the extension is installed.
const parServiceName = "datadog-agent-action"

// SetProcessManagerEnabled flips the effective process manager for ADP/PAR/PAR-executor/DDOT
// between dd-procmgrd and the native SCM services, persisting the choice into the installer
// daemon's own service environment and restarting the Datadog Agent services so the change
// takes effect. It is a no-op if the desired state already matches the current one.
func SetProcessManagerEnabled(ctx context.Context, enabled bool) error {
	if env.FromEnv().ProcessManagerEnabled == enabled {
		return nil
	}
	if err := persistProcessManagerEnabledWindows(enabled); err != nil {
		return fmt.Errorf("failed to persist process manager state: %w", err)
	}
	if err := ensureADPProcmgrConfig(enabled); err != nil {
		return fmt.Errorf("failed to configure ADP process manager config: %w", err)
	}
	if err := ensurePARExecutorProcmgrConfig(enabled); err != nil {
		return fmt.Errorf("failed to configure PAR executor process manager config: %w", err)
	}
	if err := reconcilePARServiceManager(enabled); err != nil {
		return fmt.Errorf("failed to reconcile PAR service manager: %w", err)
	}
	if err := reconcileDDOTServiceManager(enabled); err != nil {
		return fmt.Errorf("failed to reconcile DDOT service manager: %w", err)
	}
	// Unlike systemd's BindsTo/Conflicts cascade, Windows SCM services don't re-evaluate each
	// other on their own, so each affected service above is stopped/started explicitly before
	// this final restart of the base Agent services.
	return RestartDatadogAgent(ctx)
}

// reconcileDDOTServiceManager moves DDOT between dd-procmgrd and its legacy SCM service
// (registered, but left stopped, as a rollback fallback whenever the extension was installed).
func reconcileDDOTServiceManager(enabled bool) error {
	installRoot, err := resolveDatadogProgramFilesInstallRoot()
	if err != nil {
		// DDOT extension not installed: nothing to reconcile.
		return nil
	}
	if enabled {
		if err := processmanager.WriteDDOTProcmgrConfig(installRoot); err != nil {
			return err
		}
		if err := stopServiceIfExists(otelServiceName); err != nil {
			log.Warnf("DDOT: could not stop legacy service: %v", err)
		}
		processmanager.ReloadOrRestartProcmgr()
		return nil
	}
	if err := processmanager.RemoveDDOTProcmgrConfig(installRoot); err != nil {
		log.Warnf("DDOT: could not remove stale process manager config: %v", err)
	}
	processmanager.ReloadOrRestartProcmgr()
	if err := startServiceIfExists(otelServiceName); err != nil {
		log.Warnf("DDOT: could not start legacy service: %v", err)
	}
	return nil
}

// reconcilePARServiceManager moves PAR between dd-procmgrd and its native SCM service
// (datadog-agent-action, which ships unconditionally as part of the base Agent MSI, unlike
// DDOT's dormant fallback that is only registered when the extension is installed).
func reconcilePARServiceManager(enabled bool) error {
	if err := ensurePARProcmgrConfig(enabled); err != nil {
		return err
	}
	if enabled {
		if err := stopServiceIfExists(parServiceName); err != nil {
			log.Warnf("PAR: could not stop legacy service: %v", err)
		}
		processmanager.ReloadOrRestartProcmgr()
		return nil
	}
	processmanager.ReloadOrRestartProcmgr()
	if err := startServiceIfExists(parServiceName); err != nil {
		log.Warnf("PAR: could not start legacy service: %v", err)
	}
	return nil
}

// persistProcessManagerEnabledWindows writes DD_PROCESS_MANAGER_ENABLED into the "Datadog
// Installer" SCM service's Environment registry value (REG_MULTI_SZ, read by SCM on next
// service start), preserving any other entries already set on that value.
func persistProcessManagerEnabledWindows(enabled bool) error {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, env.ProcessManagerInstallerServiceRegistryKey, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return fmt.Errorf("failed to open Datadog Installer service registry key: %w", err)
	}
	defer key.Close()

	existing, _, err := key.GetStringsValue("Environment")
	if err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("failed to read Datadog Installer service environment: %w", err)
	}
	entry := fmt.Sprintf("%s=%t", processManagerEnvVar, enabled)
	filtered := slices.DeleteFunc(existing, func(s string) bool {
		return len(s) >= len(processManagerEnvVar)+1 && s[:len(processManagerEnvVar)+1] == processManagerEnvVar+"="
	})
	return key.SetStringsValue("Environment", append(filtered, entry))
}
