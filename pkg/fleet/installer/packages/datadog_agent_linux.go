// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/installinfo"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/file"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/integrations"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/packagemanager"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/selinux"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/systemd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/user"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var datadogAgentPackage = hooks{
	preInstall:  preInstallDatadogAgent,
	postInstall: postInstallDatadogAgent,
	preRemove:   preRemoveDatadogAgent,

	preStartExperiment:    preStartExperimentDatadogAgent,
	postStartExperiment:   postStartExperimentDatadogAgent,
	postPromoteExperiment: postPromoteExperimentDatadogAgent,
	preStopExperiment:     preStopExperimentDatadogAgent,
	prePromoteExperiment:  prePromoteExperimentDatadogAgent,
}

const (
	agentPackage = "datadog-agent"
	agentSymlink = "/usr/bin/datadog-agent"
)

var (
	// agentDirectories are the directories that the agent needs to function
	agentDirectories = file.Directories{
		{Path: "/etc/datadog-agent", Mode: 0755, Owner: "dd-agent", Group: "dd-agent"},
		{Path: "/var/log/datadog", Mode: 0755, Owner: "dd-agent", Group: "dd-agent"},
		{Path: "/opt/datadog-packages/run", Mode: 0755, Owner: "dd-agent", Group: "dd-agent"},
		{Path: "/opt/datadog-packages/tmp", Mode: 0755, Owner: "dd-agent", Group: "dd-agent"},
	}

	// agentConfigPermissions are the ownerships and modes that are enforced on the agent configuration files
	agentConfigPermissions = file.Permissions{
		{Path: ".", Owner: "dd-agent", Group: "dd-agent", Recursive: true},
		{Path: "managed", Owner: "root", Group: "root", Recursive: true},
		{Path: "inject", Owner: "root", Group: "root", Recursive: true},
		{Path: "compliance.d", Owner: "root", Group: "root", Recursive: true},
		{Path: "runtime-security.d", Owner: "root", Group: "root", Recursive: true},
		{Path: "system-probe.yaml", Owner: "dd-agent", Group: "dd-agent", Mode: 0440},
		{Path: "system-probe.yaml.example", Owner: "dd-agent", Group: "dd-agent", Mode: 0440},
		{Path: "security-agent.yaml", Owner: "dd-agent", Group: "dd-agent", Mode: 0440},
		{Path: "security-agent.yaml.example", Owner: "dd-agent", Group: "dd-agent", Mode: 0440},
	}

	// agentPackagePermissions are the ownerships and modes that are enforced on the agent package files
	agentPackagePermissions = file.Permissions{
		{Path: ".", Owner: "dd-agent", Group: "dd-agent", Recursive: true},
		{Path: "embedded/bin/system-probe", Owner: "root", Group: "root"},
		{Path: "embedded/bin/security-agent", Owner: "root", Group: "root"},
		{Path: "embedded/share/system-probe/ebpf", Owner: "root", Group: "root", Recursive: true},
		{Path: "embedded/share/system-probe/java", Owner: "root", Group: "root", Recursive: true},
	}
)

// setupFilesystem sets up the filesystem for the agent installation
func setupFilesystem(ctx HookContext) (err error) {
	span, ctx := ctx.StartSpan("setup_filesystem")
	defer func() {
		span.Finish(err)
	}()

	// 1. Ensure the dd-agent user and group exist
	if err = user.EnsureAgentUserAndGroup(ctx, "/opt/datadog-agent"); err != nil {
		return fmt.Errorf("failed to create dd-agent user and group: %v", err)
	}

	// 2. Ensures the installer is present in the agent package
	installerPath := filepath.Join(ctx.PackagePath, "embedded", "bin", "installer")
	if _, err := os.Stat(installerPath); os.IsNotExist(err) {
		err = installerCopy(installerPath)
		if err != nil {
			return fmt.Errorf("failed to copy installer: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to check installer: %w", err)
	}

	// 3. Ensure config/log/package directories are created and have the correct permissions
	if err = agentDirectories.Ensure(); err != nil {
		return fmt.Errorf("failed to create directories: %v", err)
	}
	if err = agentPackagePermissions.Ensure(ctx.PackagePath); err != nil {
		return fmt.Errorf("failed to set package ownerships: %v", err)
	}
	if err = agentConfigPermissions.Ensure("/etc/datadog-agent"); err != nil {
		return fmt.Errorf("failed to set config ownerships: %v", err)
	}

	// 4. Create symlinks
	if err = file.EnsureSymlink(filepath.Join(ctx.PackagePath, "bin/agent/agent"), agentSymlink); err != nil {
		return fmt.Errorf("failed to create symlink: %v", err)
	}
	if err = file.EnsureSymlink(filepath.Join(ctx.PackagePath, "embedded/bin/installer"), installerSymlink); err != nil {
		return fmt.Errorf("failed to create symlink: %v", err)
	}

	// 5. Set up SELinux permissions
	if err = selinux.SetAgentPermissions("/etc/datadog-agent", ctx.PackagePath); err != nil {
		log.Warnf("failed to set SELinux permissions: %v", err)
	}

	// 6. Handle install info
	if err = installinfo.WriteInstallInfo(string(ctx.PackageType)); err != nil {
		return fmt.Errorf("failed to write install info: %v", err)
	}
	return nil
}

// removeFilesystem cleans the filesystem
// All operations are allowed to fail
func removeFilesystem(ctx HookContext) {
	span, _ := telemetry.StartSpanFromContext(ctx, "remove_filesystem")
	defer func() {
		span.Finish(nil)
	}()

	// Remove run dir
	os.RemoveAll(filepath.Join(ctx.PackagePath, "run"))
	// Remove FIPS module
	os.Remove(filepath.Join(ctx.PackagePath, "embedded", "ssl", "fipsmodule.cnf"))
	// Remove any file related to reinstalling non-core integrations (see python-scripts/packages.py for the names)
	os.Remove(filepath.Join(ctx.PackagePath, ".pre_python_installed_packages.txt"))
	os.Remove(filepath.Join(ctx.PackagePath, ".post_python_installed_packages.txt"))
	os.Remove(filepath.Join(ctx.PackagePath, ".diff_python_installed_packages.txt"))
	// Remove install info
	installinfo.RemoveInstallInfo()
	// Remove symlinks
	os.Remove(agentSymlink)
	if target, err := os.Readlink(installerSymlink); err == nil && strings.HasPrefix(target, ctx.PackagePath) {
		os.Remove(installerSymlink)
	}
}

// installerCopy copies the current executable to the installer path
func installerCopy(path string) error {
	currentExecutable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get current executable: %w", err)
	}

	sourceFile, err := os.Open(currentExecutable)
	if err != nil {
		return fmt.Errorf("failed to open current executable: %w", err)
	}
	defer sourceFile.Close()

	err = os.MkdirAll(filepath.Dir(path), 0755)
	if err != nil {
		return fmt.Errorf("failed to create installer directory: %w", err)
	}
	destinationFile, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destinationFile.Close()

	_, err = io.Copy(destinationFile, sourceFile)
	if err != nil {
		return fmt.Errorf("failed to copy executable: %w", err)
	}

	err = destinationFile.Chmod(0755)
	if err != nil {
		return fmt.Errorf("failed to set permissions on destination file: %w", err)
	}
	return nil
}

// preInstallDatadogAgent performs pre-installation steps for the agent
func preInstallDatadogAgent(ctx HookContext) error {
	if err := stopAndRemoveAgentUnits(ctx, false, agentUnit); err != nil {
		log.Warnf("failed to stop and remove agent units: %s", err)
	}

	return packagemanager.RemovePackage(ctx, agentPackage)
}

// postInstallDatadogAgent performs post-installation steps for the agent
func postInstallDatadogAgent(ctx HookContext) (err error) {
	if err := setupFilesystem(ctx); err != nil {
		return err
	}
	if err := integrations.RestoreCustomIntegrations(ctx, ctx.PackagePath); err != nil {
		log.Warnf("failed to restore custom integrations: %s", err)
	}
	if ctx.PackageType == PackageTypeOCI {
		if err := setupAndStartAgentUnits(ctx, stableUnits, agentUnit); err != nil {
			return err
		}
	}
	return nil
}

// preRemoveDatadogAgent performs pre-removal steps for the agent
// All the steps are allowed to fail
func preRemoveDatadogAgent(ctx HookContext) error {
	if ctx.PackageType == PackageTypeOCI {
		if err := stopAndRemoveAgentUnits(ctx, true, agentUnit); err != nil {
			log.Warnf("failed to stop and remove experiment agent units: %s", err)
		}
	}

	if err := stopAndRemoveAgentUnits(ctx, false, agentUnit); err != nil {
		log.Warnf("failed to stop and remove agent units: %s", err)
	}

	if ctx.Upgrade {
		if err := integrations.SaveCustomIntegrations(ctx, ctx.PackagePath); err != nil {
			log.Warnf("failed to save custom integrations: %s", err)
		}
	}

	if err := integrations.RemoveCustomIntegrations(ctx, ctx.PackagePath); err != nil {
		log.Warnf("failed to remove custom integrations: %s\n", err.Error())
	}

	// Delete all the .pyc files. This MUST be done after using pip or any python, because executing python might generate .pyc files
	integrations.RemoveCompiledFiles(ctx.PackagePath)

	if !ctx.Upgrade {
		// Remove files not tracked by the package manager
		removeFilesystem(ctx)
	}

	return nil
}

// preStartExperimentDatadogAgent performs pre-start steps for the experiment.
// It must be executed by the stable unit before starting the experiment & before PostStartExperiment.
func preStartExperimentDatadogAgent(ctx HookContext) error {
	if err := integrations.SaveCustomIntegrations(ctx, ctx.PackagePath); err != nil {
		log.Warnf("failed to save custom integrations: %s", err)
	}
	return nil
}

// postStartExperimentDatadogAgent performs post-start steps for the experiment.
// It must be executed by the experiment unit before starting the experiment & after PreStartExperiment.
func postStartExperimentDatadogAgent(ctx HookContext) error {
	if err := setupFilesystem(ctx); err != nil {
		return err
	}

	if err := integrations.RestoreCustomIntegrations(ctx, ctx.PackagePath); err != nil {
		log.Warnf("failed to restore custom integrations: %s", err)
	}
	return setupAndStartAgentUnits(ctx, expUnits, agentExpUnit)
}

// preStopExperimentDatadogAgent performs pre-stop steps for the experiment.
// It must be executed by the experiment unit before stopping the experiment & before PostStopExperiment.
func preStopExperimentDatadogAgent(ctx HookContext) error {
	detachedCtx := context.WithoutCancel(ctx)
	return stopAndRemoveAgentUnits(detachedCtx, true, agentExpUnit) // This restarts stable units
}

// prePromoteExperimentDatadogAgent performs pre-promote steps for the experiment.
// It must be executed by the stable unit before promoting the experiment & before PostPromoteExperiment.
func prePromoteExperimentDatadogAgent(ctx HookContext) error {
	return stopAndRemoveAgentUnits(ctx, false, agentUnit)
}

// postPromoteExperimentDatadogAgent performs post-promote steps for the experiment.
// It must be executed by the experiment unit (now the new stable) before promoting the experiment & after PrePromoteExperiment.
func postPromoteExperimentDatadogAgent(ctx HookContext) error {
	if err := setupFilesystem(ctx); err != nil {
		return err
	}

	detachedCtx := context.WithoutCancel(ctx)
	if err := setupAndStartAgentUnits(detachedCtx, stableUnits, agentUnit); err != nil {
		return err
	}
	return stopAndRemoveAgentUnits(detachedCtx, true, agentExpUnit)
}

const (
	agentUnit             = "datadog-agent.service"
	installerAgentUnit    = "datadog-agent-installer.service"
	traceAgentUnit        = "datadog-agent-trace.service"
	processAgentUnit      = "datadog-agent-process.service"
	systemProbeUnit       = "datadog-agent-sysprobe.service"
	securityAgentUnit     = "datadog-agent-security.service"
	agentExpUnit          = "datadog-agent-exp.service"
	installerAgentExpUnit = "datadog-agent-installer-exp.service"
	traceAgentExpUnit     = "datadog-agent-trace-exp.service"
	processAgentExpUnit   = "datadog-agent-process-exp.service"
	systemProbeExpUnit    = "datadog-agent-sysprobe-exp.service"
	securityAgentExpUnit  = "datadog-agent-security-exp.service"
)

var (
	stableUnits = []string{
		agentUnit,
		installerAgentUnit,
		traceAgentUnit,
		processAgentUnit,
		systemProbeUnit,
		securityAgentUnit,
	}
	expUnits = []string{
		agentExpUnit,
		installerAgentExpUnit,
		traceAgentExpUnit,
		processAgentExpUnit,
		systemProbeExpUnit,
		securityAgentExpUnit,
	}
)

func stopAndRemoveAgentUnits(ctx context.Context, experiment bool, mainUnit string) error {
	units, err := systemd.ListOnDiskAgentUnits(experiment)
	if err != nil {
		return fmt.Errorf("failed to list agent units: %v", err)
	}

	for _, unit := range units {
		if err := systemd.StopUnit(ctx, unit); err != nil {
			return err
		}
	}

	if err := systemd.DisableUnit(ctx, mainUnit); err != nil {
		return err
	}

	for _, unit := range units {
		if err := systemd.RemoveUnit(ctx, unit); err != nil {
			return err
		}
	}
	return nil
}

func setupAndStartAgentUnits(ctx context.Context, units []string, mainUnit string) error {
	for _, unit := range units {
		if err := systemd.WriteEmbeddedUnit(ctx, unit); err != nil {
			return fmt.Errorf("failed to load %s: %v", unit, err)
		}
	}

	if err := systemd.Reload(ctx); err != nil {
		return fmt.Errorf("failed to reload systemd daemon: %v", err)
	}

	// enabling the core agent unit only is enough as others are triggered by it
	if err := systemd.EnableUnit(ctx, mainUnit); err != nil {
		return fmt.Errorf("failed to enable %s: %v", mainUnit, err)
	}

	_, err := os.Stat("/etc/datadog-agent/datadog.yaml")
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to check if /etc/datadog-agent/datadog.yaml exists: %v", err)
	} else if os.IsNotExist(err) {
		// this is expected during a fresh install with the install script / ansible / chef / etc...
		// the config is populated afterwards by the install method and the agent is restarted
		return nil
	}
	if err = systemd.StartUnit(ctx, mainUnit); err != nil {
		return err
	}
	return nil
}
