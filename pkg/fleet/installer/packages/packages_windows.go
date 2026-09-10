// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/processmanager"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// packagesHooks is a map of package names to their hooks
	packagesHooks = map[string]hooks{
		"datadog-agent":              datadogAgentPackage,
		"datadog-apm-library-dotnet": apmLibraryDotnetPackage,
		"datadog-apm-inject":         apmInjectPackage,
		"datadog-agent-ddot":         datadogAgentDDOTPackage,
	}

	// packageCommands is a map of package names to their command handlers
	packageCommands = map[string]PackageCommandHandler{
		"datadog-agent": runDatadogAgentPackageCommand,
	}

	// AsyncPreRemoveHooks is called before a package is removed from the disk.
	// It can block the removal of the package files until a condition is met without blocking
	// the rest of the uninstall or upgrade process.
	// Today this is only useful for the dotnet tracer on windows and generally *SHOULD BE AVOIDED*.
	AsyncPreRemoveHooks = map[string]repository.PreRemoveHook{
		"datadog-apm-library-dotnet": asyncPreRemoveHookAPMLibraryDotnet,
	}
)

// parServiceName is PAR's native SCM service (cmd/agent/subcommands/run/dependent_services_windows.go),
// distinct from otelServiceName. It ships as part of the base Agent MSI, so it is always
// present to stop/start directly, unlike DDOT's dormant fallback which is only registered when
// the extension is installed.
const parServiceName = "datadog-agent-action"

// SetProcessManager flips the effective process manager for ADP/PAR/PAR-executor/DDOT
// between dd-procmgrd and the native SCM services, restarting the Datadog Agent services so the
// change takes effect. It is a no-op if the desired state already matches the current one.
func SetProcessManager(ctx context.Context, enabled bool) error {
	if env.FromEnv().ProcessManagerEnabled == enabled {
		return nil
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
