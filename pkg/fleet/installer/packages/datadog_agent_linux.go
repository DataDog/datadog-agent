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
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/systemd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/sysvinit"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/upstart"
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

	// agentPackageUninstallPaths are the paths that are deleted during an uninstall
	agentPackageUninstallPaths = file.Paths{
		"embedded/ssl/fipsmodule.cnf",
		"run",
		".pre_python_installed_packages.txt",
		".post_python_installed_packages.txt",
		".diff_python_installed_packages.txt",
	}

	// agentConfigUninstallPaths are the files that are deleted during an uninstall
	agentConfigUninstallPaths = file.Paths{
		"install_info",
		"install.json",
	}

	// agentService are the services that are part of the agent deb/rpm package
	agentService = datadogAgentService{
		SystemdMainUnit:     "datadog-agent.service",
		SystemdUnits:        []string{"datadog-agent.service", "datadog-agent-trace.service", "datadog-agent-process.service", "datadog-agent-sysprobe.service", "datadog-agent-security.service"},
		UpstartMainService:  "datadog-agent",
		UpstartServices:     []string{"datadog-agent", "datadog-agent-trace", "datadog-agent-process", "datadog-agent-sysprobe", "datadog-agent-security"},
		SysvinitMainService: "datadog-agent",
		SysvinitServices:    []string{"datadog-agent", "datadog-agent-trace", "datadog-agent-process", "datadog-agent-security"},
	}

	// agentServiceOCI are the services that are part of the agent oci package
	// FIXME: Will be merged with agentService once we support the installer daemon on deb/rpm
	agentServiceOCI = datadogAgentServiceOCI{
		SystemdMainUnitStable: "datadog-agent.service",
		SystemdMainUnitExp:    "datadog-agent-exp.service",
		SystemdUnitsStable:    []string{"datadog-agent.service", "datadog-agent-installer.service", "datadog-agent-trace.service", "datadog-agent-process.service", "datadog-agent-sysprobe.service", "datadog-agent-security.service"},
		SystemdUnitsExp:       []string{"datadog-agent-exp.service", "datadog-agent-installer-exp.service", "datadog-agent-trace-exp.service", "datadog-agent-process-exp.service", "datadog-agent-sysprobe-exp.service", "datadog-agent-security-exp.service"},
	}
)

// installFilesystem sets up the filesystem for the agent installation
func installFilesystem(ctx HookContext) (err error) {
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

// uninstallFilesystem cleans the filesystem by removing various temporary files, symlinks and installation metadata
func uninstallFilesystem(ctx HookContext) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "remove_filesystem")
	defer func() {
		span.Finish(err)
	}()

	err = agentPackageUninstallPaths.EnsureAbsent(ctx.PackagePath)
	if err != nil {
		return fmt.Errorf("failed to remove package paths: %w", err)
	}
	err = agentConfigUninstallPaths.EnsureAbsent("/etc/datadog-agent")
	if err != nil {
		return fmt.Errorf("failed to remove config paths: %w", err)
	}
	err = file.EnsureSymlinkAbsent(agentSymlink)
	if err != nil {
		return fmt.Errorf("failed to remove agent symlink: %w", err)
	}

	installerTarget, err := os.Readlink(installerSymlink)
	if err != nil {
		return fmt.Errorf("failed to read installer symlink: %w", err)
	}
	if strings.HasPrefix(installerTarget, ctx.PackagePath) {
		err = file.EnsureSymlinkAbsent(installerSymlink)
		if err != nil {
			return fmt.Errorf("failed to remove installer symlink: %w", err)
		}
	}
	return nil
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
	if err := agentServiceOCI.StopStable(ctx); err != nil {
		log.Warnf("failed to stop stable unit: %s", err)
	}
	if err := agentServiceOCI.DisableStable(ctx); err != nil {
		log.Warnf("failed to disable stable unit: %s", err)
	}
	if err := agentServiceOCI.RemoveStable(ctx); err != nil {
		log.Warnf("failed to remove stable unit: %s", err)
	}
	return packagemanager.RemovePackage(ctx, agentPackage)
}

// postInstallDatadogAgent performs post-installation steps for the agent
func postInstallDatadogAgent(ctx HookContext) (err error) {
	if err := installFilesystem(ctx); err != nil {
		return err
	}
	if err := integrations.RestoreCustomIntegrations(ctx, ctx.PackagePath); err != nil {
		log.Warnf("failed to restore custom integrations: %s", err)
	}
	switch ctx.PackageType {
	case PackageTypeDEB, PackageTypeRPM:
		if err := agentService.Enable(ctx); err != nil {
			log.Warnf("failed to install agent service: %s", err)
		}
		if err := agentService.Restart(ctx); err != nil {
			log.Warnf("failed to restart agent: %s", err)
		}
	case PackageTypeOCI:
		if err := agentServiceOCI.WriteStable(ctx); err != nil {
			return fmt.Errorf("failed to write stable units: %s", err)
		}
		if err := agentServiceOCI.EnableStable(ctx); err != nil {
			return fmt.Errorf("failed to install stable unit: %s", err)
		}
		if err := agentServiceOCI.RestartStable(ctx); err != nil {
			return fmt.Errorf("failed to restart stable unit: %s", err)
		}
	}
	return nil
}

// preRemoveDatadogAgent performs pre-removal steps for the agent
// All the steps are allowed to fail
func preRemoveDatadogAgent(ctx HookContext) error {
	switch ctx.PackageType {
	case PackageTypeDEB, PackageTypeRPM:
		if err := agentService.Stop(ctx); err != nil {
			log.Warnf("failed to stop agent service: %s", err)
		}
		if err := agentService.Disable(ctx); err != nil {
			log.Warnf("failed to disable agent service: %s", err)
		}
	case PackageTypeOCI:
		err := agentServiceOCI.StopExperiment(ctx)
		if err != nil {
			log.Warnf("failed to stop experiment unit: %s", err)
		}
		err = agentServiceOCI.RemoveExperiment(ctx)
		if err != nil {
			log.Warnf("failed to remove experiment unit: %s", err)
		}
		err = agentServiceOCI.StopStable(ctx)
		if err != nil {
			log.Warnf("failed to stop stable unit: %s", err)
		}
		err = agentServiceOCI.DisableStable(ctx)
		if err != nil {
			log.Warnf("failed to disable stable unit: %s", err)
		}
		err = agentServiceOCI.RemoveStable(ctx)
		if err != nil {
			log.Warnf("failed to remove stable unit: %s", err)
		}
	}
	switch ctx.Upgrade {
	case false:
		if err := integrations.RemoveCustomIntegrations(ctx, ctx.PackagePath); err != nil {
			log.Warnf("failed to remove custom integrations: %s\n", err.Error())
		}
		if err := integrations.RemoveCompiledFiles(ctx.PackagePath); err != nil {
			log.Warnf("failed to remove compiled files: %s", err)
		}
		if err := uninstallFilesystem(ctx); err != nil {
			log.Warnf("failed to uninstall filesystem: %s", err)
		}
	case true:
		if err := integrations.SaveCustomIntegrations(ctx, ctx.PackagePath); err != nil {
			log.Warnf("failed to save custom integrations: %s", err)
		}
		if err := integrations.RemoveCustomIntegrations(ctx, ctx.PackagePath); err != nil {
			log.Warnf("failed to remove custom integrations: %s\n", err.Error())
		}
		if err := integrations.RemoveCompiledFiles(ctx.PackagePath); err != nil {
			log.Warnf("failed to remove compiled files: %s", err)
		}
	}
	return nil
}

// preStartExperimentDatadogAgent performs pre-start steps for the experiment.
// It must be executed by the stable unit before starting the experiment & before PostStartExperiment.
func preStartExperimentDatadogAgent(ctx HookContext) error {
	err := agentServiceOCI.RemoveExperiment(ctx)
	if err != nil {
		return fmt.Errorf("failed to remove experiment units: %s", err)
	}
	if err := integrations.SaveCustomIntegrations(ctx, ctx.PackagePath); err != nil {
		log.Warnf("failed to save custom integrations: %s", err)
	}
	return nil
}

// postStartExperimentDatadogAgent performs post-start steps for the experiment.
// It must be executed by the experiment unit before starting the experiment & after PreStartExperiment.
func postStartExperimentDatadogAgent(ctx HookContext) error {
	if err := installFilesystem(ctx); err != nil {
		return err
	}
	if err := integrations.RestoreCustomIntegrations(ctx, ctx.PackagePath); err != nil {
		log.Warnf("failed to restore custom integrations: %s", err)
	}
	if err := agentServiceOCI.WriteExperiment(ctx); err != nil {
		return err
	}
	if err := agentServiceOCI.StartExperiment(ctx); err != nil {
		return err
	}
	return nil
}

// preStopExperimentDatadogAgent performs pre-stop steps for the experiment.
// It must be executed by the experiment unit before stopping the experiment & before PostStopExperiment.
func preStopExperimentDatadogAgent(ctx HookContext) error {
	detachedCtx := context.WithoutCancel(ctx.Context)
	ctx.Context = detachedCtx
	if err := agentServiceOCI.StopExperiment(ctx); err != nil {
		return fmt.Errorf("failed to stop experiment unit: %s", err)
	}
	if err := agentServiceOCI.RemoveExperiment(ctx); err != nil {
		return fmt.Errorf("failed to remove experiment unit: %s", err)
	}
	return nil
}

// prePromoteExperimentDatadogAgent performs pre-promote steps for the experiment.
// It must be executed by the stable unit before promoting the experiment & before PostPromoteExperiment.
func prePromoteExperimentDatadogAgent(ctx HookContext) error {
	if err := agentServiceOCI.StopStable(ctx); err != nil {
		return fmt.Errorf("failed to stop stable unit: %s", err)
	}
	if err := agentServiceOCI.DisableStable(ctx); err != nil {
		return fmt.Errorf("failed to disable stable unit: %s", err)
	}
	if err := agentServiceOCI.RemoveStable(ctx); err != nil {
		return fmt.Errorf("failed to remove stable unit: %s", err)
	}
	return nil
}

// postPromoteExperimentDatadogAgent performs post-promote steps for the experiment.
// It must be executed by the experiment unit (now the new stable) before promoting the experiment & after PrePromoteExperiment.
func postPromoteExperimentDatadogAgent(ctx HookContext) error {
	if err := installFilesystem(ctx); err != nil {
		return err
	}
	detachedCtx := context.WithoutCancel(ctx.Context)
	ctx.Context = detachedCtx
	err := agentServiceOCI.WriteStable(ctx)
	if err != nil {
		return err
	}
	err = agentServiceOCI.EnableStable(ctx)
	if err != nil {
		return err
	}
	err = agentServiceOCI.RestartStable(ctx)
	if err != nil {
		return err
	}
	return nil
}

type datadogAgentService struct {
	SystemdMainUnit     string
	SystemdUnits        []string
	UpstartMainService  string
	UpstartServices     []string
	SysvinitMainService string
	SysvinitServices    []string
}

// Enable installs / enables the agent service
func (s *datadogAgentService) Enable(ctx HookContext) error {
	serviceManagerType := service.GetServiceManagerType()
	if serviceManagerType == service.SysvinitType && ctx.PackageType != PackageTypeDEB {
		return fmt.Errorf("sysvinit is only supported on Debian")
	}
	switch serviceManagerType {
	case service.SystemdType:
		return systemd.EnableUnit(ctx, s.SystemdMainUnit)
	case service.UpstartType:
		// Nothing to do, this is defined directly in the upstart job file
		return nil
	case service.SysvinitType:
		for _, service := range s.SysvinitServices {
			err := sysvinit.Install(ctx, service)
			if err != nil {
				return fmt.Errorf("failed to install %s: %v", service, err)
			}
		}
		return nil
	}
	return fmt.Errorf("unsupported service manager type: %s", serviceManagerType)
}

// Disable disables the agent service
func (s *datadogAgentService) Disable(ctx HookContext) error {
	serviceManagerType := service.GetServiceManagerType()
	if serviceManagerType == service.SysvinitType && ctx.PackageType != PackageTypeDEB {
		return fmt.Errorf("sysvinit is only supported on Debian")
	}
	switch serviceManagerType {
	case service.SystemdType:
		for _, unit := range s.SystemdUnits {
			err := systemd.DisableUnit(ctx, unit)
			if err != nil {
				return fmt.Errorf("failed to disable %s: %v", unit, err)
			}
		}
		return nil
	case service.UpstartType:
		// Nothing to do, this is defined directly in the upstart job file
		return nil
	case service.SysvinitType:
		for _, service := range s.SysvinitServices {
			err := sysvinit.Remove(ctx, service)
			if err != nil {
				return fmt.Errorf("failed to remove %s: %v", service, err)
			}
		}
		return nil
	}
	return fmt.Errorf("unsupported service manager type: %s", serviceManagerType)
}

// Restart restarts the agent service. It will only attempt to restart if the config exists.
func (s *datadogAgentService) Restart(ctx HookContext) error {
	_, err := os.Stat("/etc/datadog-agent/datadog.yaml")
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to check if /etc/datadog-agent/datadog.yaml exists: %v", err)
	} else if os.IsNotExist(err) {
		// this is expected during a fresh install with the install script / ansible / chef / etc...
		// the config is populated afterwards by the install method and the agent is restarted
		return nil
	}
	serviceManagerType := service.GetServiceManagerType()
	if serviceManagerType == service.SysvinitType && ctx.PackageType != PackageTypeDEB {
		return fmt.Errorf("sysvinit is only supported on Debian")
	}
	switch serviceManagerType {
	case service.UpstartType:
		return upstart.Restart(ctx, s.UpstartMainService)
	case service.SysvinitType:
		return sysvinit.Restart(ctx, s.SysvinitMainService)
	case service.SystemdType:
		return systemd.RestartUnit(ctx, s.SystemdMainUnit)
	}
	return fmt.Errorf("unsupported service manager type: %s", serviceManagerType)
}

// Stop stops the agent service
func (s *datadogAgentService) Stop(ctx HookContext) error {
	serviceManagerType := service.GetServiceManagerType()
	if serviceManagerType == service.SysvinitType && ctx.PackageType != PackageTypeDEB {
		return fmt.Errorf("sysvinit is only supported on Debian")
	}
	switch serviceManagerType {
	case service.SystemdType:
		for _, unit := range s.SystemdUnits {
			err := systemd.StopUnit(ctx, unit)
			if err != nil {
				return fmt.Errorf("failed to stop %s: %v", unit, err)
			}
		}
		return nil
	case service.UpstartType:
		for _, service := range s.UpstartServices {
			err := upstart.Stop(ctx, service)
			if err != nil {
				return fmt.Errorf("failed to stop %s: %v", service, err)
			}
		}
		return nil
	case service.SysvinitType:
		for _, service := range s.SysvinitServices {
			err := sysvinit.Stop(ctx, service)
			if err != nil {
				return fmt.Errorf("failed to stop %s: %v", service, err)
			}
		}
		return nil
	}
	return fmt.Errorf("unsupported service manager type: %s", serviceManagerType)
}

type datadogAgentServiceOCI struct {
	SystemdMainUnitStable string
	SystemdMainUnitExp    string
	SystemdUnitsStable    []string
	SystemdUnitsExp       []string
}

// EnableStable enables the stable unit
func (s *datadogAgentServiceOCI) EnableStable(ctx HookContext) error {
	return systemd.EnableUnit(ctx, s.SystemdMainUnitStable)
}

// DisableStable disables the stable unit
func (s *datadogAgentServiceOCI) DisableStable(ctx HookContext) error {
	return systemd.DisableUnit(ctx, s.SystemdMainUnitStable)
}

// RestartStable restarts the stable unit. It will only attempt to restart if the config exists.
func (s *datadogAgentServiceOCI) RestartStable(ctx HookContext) error {
	_, err := os.Stat("/etc/datadog-agent/datadog.yaml")
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to check if /etc/datadog-agent/datadog.yaml exists: %v", err)
	} else if os.IsNotExist(err) {
		// this is expected during a fresh install with the install script / ansible / chef / etc...
		// the config is populated afterwards by the install method and the agent is restarted
		return nil
	}
	return systemd.RestartUnit(ctx, s.SystemdMainUnitStable)
}

// StopStable stops the stable units
func (s *datadogAgentServiceOCI) StopStable(ctx HookContext) error {
	return systemd.StopUnits(ctx, s.SystemdUnitsStable...)
}

// RemoveStable removes the stable units
func (s *datadogAgentServiceOCI) RemoveStable(ctx HookContext) error {
	return systemd.RemoveUnits(ctx, s.SystemdUnitsStable...)
}

// StartExperiment starts the experiment unit
func (s *datadogAgentServiceOCI) StartExperiment(ctx HookContext) error {
	return systemd.StartUnit(ctx, s.SystemdMainUnitExp)
}

// StopExperiment stops the experiment units
func (s *datadogAgentServiceOCI) StopExperiment(ctx HookContext) error {
	return systemd.StopUnits(ctx, s.SystemdMainUnitExp)
}

// RemoveExperiment removes the experiment units from the disk
func (s *datadogAgentServiceOCI) RemoveExperiment(ctx HookContext) error {
	return systemd.RemoveUnits(ctx, s.SystemdUnitsExp...)
}

// WriteStableUnits writes the stable units to the system and reloads the systemd daemon
func (s *datadogAgentServiceOCI) WriteStable(ctx HookContext) error {
	return systemd.WriteEmbeddedUnitsAndReload(ctx, s.SystemdUnitsStable...)
}

// WriteExperiment writes the experiment units to the system and reloads the systemd daemon
func (s *datadogAgentServiceOCI) WriteExperiment(ctx HookContext) error {
	return systemd.WriteEmbeddedUnitsAndReload(ctx, s.SystemdUnitsExp...)
}
