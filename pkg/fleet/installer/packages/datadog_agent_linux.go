// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/installinfo"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
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

	postStartConfigExperiment:   postStartConfigExperimentDatadogAgent,
	preStopConfigExperiment:     preStopConfigExperimentDatadogAgent,
	postPromoteConfigExperiment: postPromoteConfigExperimentDatadogAgent,
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

	// agentServiceOCI are the services that are part of the agent package
	agentService = datadogAgentService{
		SystemdMainUnitStable: "datadog-agent.service",
		SystemdMainUnitExp:    "datadog-agent-exp.service",
		SystemdUnitsStable:    []string{"datadog-agent.service", "datadog-agent-installer.service", "datadog-agent-trace.service", "datadog-agent-process.service", "datadog-agent-sysprobe.service", "datadog-agent-security.service"},
		SystemdUnitsExp:       []string{"datadog-agent-exp.service", "datadog-agent-installer-exp.service", "datadog-agent-trace-exp.service", "datadog-agent-process-exp.service", "datadog-agent-sysprobe-exp.service", "datadog-agent-security-exp.service"},

		UpstartMainService: "datadog-agent",
		UpstartServices:    []string{"datadog-agent", "datadog-agent-trace", "datadog-agent-process", "datadog-agent-sysprobe", "datadog-agent-security"},

		SysvinitMainService: "datadog-agent",
		SysvinitServices:    []string{"datadog-agent", "datadog-agent-trace", "datadog-agent-process", "datadog-agent-security"},
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

	// 2. Ensure config/log/package directories are created and have the correct permissions
	if err = agentDirectories.Ensure(); err != nil {
		return fmt.Errorf("failed to create directories: %v", err)
	}
	if err = agentPackagePermissions.Ensure(ctx.PackagePath); err != nil {
		return fmt.Errorf("failed to set package ownerships: %v", err)
	}
	if err = agentConfigPermissions.Ensure("/etc/datadog-agent"); err != nil {
		return fmt.Errorf("failed to set config ownerships: %v", err)
	}

	// 3. Create symlinks
	if err = file.EnsureSymlink(filepath.Join(ctx.PackagePath, "bin/agent/agent"), agentSymlink); err != nil {
		return fmt.Errorf("failed to create symlink: %v", err)
	}
	if err = file.EnsureSymlink(filepath.Join(ctx.PackagePath, "embedded/bin/installer"), installerSymlink); err != nil {
		return fmt.Errorf("failed to create symlink: %v", err)
	}

	// 4. Set up SELinux permissions
	if err = selinux.SetAgentPermissions("/etc/datadog-agent", ctx.PackagePath); err != nil {
		log.Warnf("failed to set SELinux permissions: %v", err)
	}

	// 5. Handle install info
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

// preInstallDatadogAgent performs pre-installation steps for the agent
func preInstallDatadogAgent(ctx HookContext) error {
	if err := agentService.StopStable(ctx); err != nil {
		log.Warnf("failed to stop stable unit: %s", err)
	}
	if err := agentService.DisableStable(ctx); err != nil {
		log.Warnf("failed to disable stable unit: %s", err)
	}
	if err := agentService.RemoveStable(ctx); err != nil {
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
	if err := agentService.WriteStable(ctx); err != nil {
		return fmt.Errorf("failed to write stable units: %s", err)
	}
	if err := agentService.EnableStable(ctx); err != nil {
		return fmt.Errorf("failed to install stable unit: %s", err)
	}
	if err := agentService.RestartStable(ctx); err != nil {
		return fmt.Errorf("failed to restart stable unit: %s", err)
	}
	return nil
}

// preRemoveDatadogAgent performs pre-removal steps for the agent
// All the steps are allowed to fail
func preRemoveDatadogAgent(ctx HookContext) error {
	err := agentService.StopExperiment(ctx)
	if err != nil {
		log.Warnf("failed to stop experiment unit: %s", err)
	}
	err = agentService.RemoveExperiment(ctx)
	if err != nil {
		log.Warnf("failed to remove experiment unit: %s", err)
	}
	err = agentService.StopStable(ctx)
	if err != nil {
		log.Warnf("failed to stop stable unit: %s", err)
	}
	err = agentService.DisableStable(ctx)
	if err != nil {
		log.Warnf("failed to disable stable unit: %s", err)
	}
	err = agentService.RemoveStable(ctx)
	if err != nil {
		log.Warnf("failed to remove stable unit: %s", err)
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
	err := agentService.RemoveExperiment(ctx)
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
	if err := agentService.WriteExperiment(ctx); err != nil {
		return err
	}
	if err := agentService.StartExperiment(ctx); err != nil {
		return err
	}
	return nil
}

// preStopExperimentDatadogAgent performs pre-stop steps for the experiment.
// It must be executed by the experiment unit before stopping the experiment & before PostStopExperiment.
func preStopExperimentDatadogAgent(ctx HookContext) error {
	detachedCtx := context.WithoutCancel(ctx.Context)
	ctx.Context = detachedCtx
	if err := agentService.StopExperiment(ctx); err != nil {
		return fmt.Errorf("failed to stop experiment unit: %s", err)
	}
	if err := agentService.RemoveExperiment(ctx); err != nil {
		return fmt.Errorf("failed to remove experiment unit: %s", err)
	}
	return nil
}

// prePromoteExperimentDatadogAgent performs pre-promote steps for the experiment.
// It must be executed by the stable unit before promoting the experiment & before PostPromoteExperiment.
func prePromoteExperimentDatadogAgent(ctx HookContext) error {
	if err := agentService.StopStable(ctx); err != nil {
		return fmt.Errorf("failed to stop stable unit: %s", err)
	}
	if err := agentService.DisableStable(ctx); err != nil {
		return fmt.Errorf("failed to disable stable unit: %s", err)
	}
	if err := agentService.RemoveStable(ctx); err != nil {
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
	err := agentService.WriteStable(ctx)
	if err != nil {
		return err
	}
	err = agentService.EnableStable(ctx)
	if err != nil {
		return err
	}
	err = agentService.RestartStable(ctx)
	if err != nil {
		return err
	}
	return nil
}

// postStartConfigExperimentDatadogAgent performs post-start steps for the config experiment.
func postStartConfigExperimentDatadogAgent(ctx HookContext) error {
	if err := agentService.WriteExperiment(ctx); err != nil {
		return err
	}
	if err := agentService.StartExperiment(ctx); err != nil {
		return err
	}
	return nil
}

// preStopConfigExperimentDatadogAgent performs pre-stop steps for the config experiment.
func preStopConfigExperimentDatadogAgent(ctx HookContext) error {
	detachedCtx := context.WithoutCancel(ctx.Context)
	ctx.Context = detachedCtx
	if err := agentService.StopExperiment(ctx); err != nil {
		return fmt.Errorf("failed to stop experiment unit: %s", err)
	}
	if err := agentService.RemoveExperiment(ctx); err != nil {
		return fmt.Errorf("failed to remove experiment unit: %s", err)
	}
	return nil
}

// postPromoteConfigExperimentDatadogAgent performs post-promote steps for the config experiment.
func postPromoteConfigExperimentDatadogAgent(ctx HookContext) error {
	detachedCtx := context.WithoutCancel(ctx.Context)
	ctx.Context = detachedCtx
	err := agentService.RestartStable(ctx)
	if err != nil {
		return err
	}
	return nil
}

type datadogAgentService struct {
	SystemdMainUnitStable string
	SystemdMainUnitExp    string
	SystemdUnitsStable    []string
	SystemdUnitsExp       []string

	UpstartMainService string
	UpstartServices    []string

	SysvinitMainService string
	SysvinitServices    []string
}

func (s *datadogAgentService) checkPlatformSupport(ctx HookContext) error {
	switch service.GetServiceManagerType() {
	case service.SystemdType:
		return nil
	case service.UpstartType:
		if ctx.PackageType != PackageTypeDEB && ctx.PackageType != PackageTypeRPM {
			return fmt.Errorf("upstart is only supported in DEB and RPM packages")
		}
	case service.SysvinitType:
		if ctx.PackageType != PackageTypeDEB {
			return fmt.Errorf("sysvinit is only supported in DEB packages")
		}
	default:
		return fmt.Errorf("could not determine service manager type, platform is not supported")
	}
	return nil
}

// EnableStable enables the stable unit
func (s *datadogAgentService) EnableStable(ctx HookContext) error {
	if err := s.checkPlatformSupport(ctx); err != nil {
		return err
	}
	switch service.GetServiceManagerType() {
	case service.SystemdType:
		return systemd.EnableUnit(ctx, s.SystemdMainUnitStable)
	case service.UpstartType:
		return nil // Nothing to do, this is defined directly in the upstart job file
	case service.SysvinitType:
		return sysvinit.InstallAll(ctx, s.SysvinitServices...)
	default:
		return fmt.Errorf("unsupported service manager")
	}
}

// DisableStable disables the stable unit
func (s *datadogAgentService) DisableStable(ctx HookContext) error {
	if err := s.checkPlatformSupport(ctx); err != nil {
		return err
	}
	switch service.GetServiceManagerType() {
	case service.SystemdType:
		return systemd.DisableUnits(ctx, s.SystemdUnitsStable...)
	case service.UpstartType:
		return nil // Nothing to do, this is defined directly in the upstart job file
	case service.SysvinitType:
		return sysvinit.RemoveAll(ctx, s.SysvinitServices...)
	default:
		return fmt.Errorf("unsupported service manager")
	}
}

// RestartStable restarts the stable unit. It will only attempt to restart if the config exists.
func (s *datadogAgentService) RestartStable(ctx HookContext) error {
	if err := s.checkPlatformSupport(ctx); err != nil {
		return err
	}
	present, err := isAgentConfigFilePresent()
	if err != nil {
		return fmt.Errorf("failed to check if /etc/datadog-agent/datadog.yaml exists: %v", err)
	}
	if !present {
		return nil
	}
	switch service.GetServiceManagerType() {
	case service.SystemdType:
		return systemd.RestartUnit(ctx, s.SystemdMainUnitStable)
	case service.UpstartType:
		return upstart.Restart(ctx, s.UpstartMainService)
	case service.SysvinitType:
		return sysvinit.Restart(ctx, s.SysvinitMainService)
	default:
		return fmt.Errorf("unsupported service manager")
	}
}

// StopStable stops the stable units
func (s *datadogAgentService) StopStable(ctx HookContext) error {
	if err := s.checkPlatformSupport(ctx); err != nil {
		return err
	}
	switch service.GetServiceManagerType() {
	case service.SystemdType:
		return systemd.StopUnits(ctx, reverseStringSlice(s.SystemdUnitsStable)...)
	case service.UpstartType:
		return upstart.StopAll(ctx, reverseStringSlice(s.UpstartServices)...)
	case service.SysvinitType:
		return sysvinit.StopAll(ctx, reverseStringSlice(s.SysvinitServices)...)
	default:
		return fmt.Errorf("unsupported service manager")
	}
}

// WriteStable writes the stable units to the system and reloads the systemd daemon
func (s *datadogAgentService) WriteStable(ctx HookContext) error {
	if err := s.checkPlatformSupport(ctx); err != nil {
		return err
	}
	switch service.GetServiceManagerType() {
	case service.SystemdType:
		return writeEmbeddedUnitsAndReload(ctx, s.SystemdUnitsStable...)
	case service.UpstartType:
		return nil // Nothing to do, files are embedded in the package
	case service.SysvinitType:
		return nil // Nothing to do, files are embedded in the package
	}
	return fmt.Errorf("unsupported service manager")
}

// RemoveStable removes the stable units
func (s *datadogAgentService) RemoveStable(ctx HookContext) error {
	if err := s.checkPlatformSupport(ctx); err != nil {
		return err
	}
	switch service.GetServiceManagerType() {
	case service.SystemdType:
		return removeUnits(ctx, s.SystemdUnitsStable...)
	case service.UpstartType:
		return nil // Nothing to do, files are embedded in the package
	case service.SysvinitType:
		return nil // Nothing to do, files are embedded in the package
	}
	return fmt.Errorf("unsupported service manager")
}

// StartExperiment starts the experiment unit
func (s *datadogAgentService) StartExperiment(ctx HookContext) error {
	if err := s.checkPlatformSupport(ctx); err != nil {
		return err
	}
	switch service.GetServiceManagerType() {
	case service.SystemdType:
		return systemd.StartUnit(ctx, s.SystemdMainUnitExp)
	case service.UpstartType:
		return fmt.Errorf("experiments are not supported on upstart")
	case service.SysvinitType:
		return fmt.Errorf("experiments are not supported on sysvinit")
	}
	return fmt.Errorf("unsupported service manager")
}

// StopExperiment stops the experiment units
func (s *datadogAgentService) StopExperiment(ctx HookContext) error {
	if err := s.checkPlatformSupport(ctx); err != nil {
		return err
	}
	switch service.GetServiceManagerType() {
	case service.SystemdType:
		return systemd.StopUnits(ctx, s.SystemdMainUnitExp)
	case service.UpstartType:
		return nil // Experiments are not supported on upstart
	case service.SysvinitType:
		return nil // Experiments are not supported on sysvinit
	}
	return fmt.Errorf("unsupported service manager")
}

// WriteExperiment writes the experiment units to the system and reloads the systemd daemon
func (s *datadogAgentService) WriteExperiment(ctx HookContext) error {
	if err := s.checkPlatformSupport(ctx); err != nil {
		return err
	}
	switch service.GetServiceManagerType() {
	case service.SystemdType:
		return writeEmbeddedUnitsAndReload(ctx, s.SystemdUnitsExp...)
	case service.UpstartType:
		return fmt.Errorf("experiments are not supported on upstart")
	case service.SysvinitType:
		return fmt.Errorf("experiments are not supported on sysvinit")
	}
	return fmt.Errorf("unsupported service manager")
}

// RemoveExperiment removes the experiment units from the disk
func (s *datadogAgentService) RemoveExperiment(ctx HookContext) error {
	if err := s.checkPlatformSupport(ctx); err != nil {
		return err
	}
	switch service.GetServiceManagerType() {
	case service.SystemdType:
		return removeUnits(ctx, s.SystemdUnitsExp...)
	case service.UpstartType:
		return nil // Experiments are not supported on upstart
	case service.SysvinitType:
		return nil // Experiments are not supported on sysvinit
	}
	return fmt.Errorf("unsupported service manager")
}

// isAgentConfigFilePresent checks if the agent config file exists
func isAgentConfigFilePresent() (bool, error) {
	_, err := os.Stat("/etc/datadog-agent/datadog.yaml")
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("failed to check if /etc/datadog-agent/datadog.yaml exists: %v", err)
	}
	return !os.IsNotExist(err), nil
}

const (
	ociUnitsPath = "/etc/systemd/system"
	debUnitsPath = "/lib/systemd/system"
	rpmUnitsPath = "/usr/lib/systemd/system"
)

func removeUnits(ctx HookContext, units ...string) error {
	var unitsPath string
	switch ctx.PackageType {
	case PackageTypeDEB:
		unitsPath = debUnitsPath
	case PackageTypeRPM:
		unitsPath = rpmUnitsPath
	case PackageTypeOCI:
		unitsPath = ociUnitsPath
	}
	for _, unit := range units {
		err := os.Remove(filepath.Join(unitsPath, unit))
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove unit: %v", err)
		}
	}
	return nil
}

func writeEmbeddedUnitsAndReload(ctx HookContext, units ...string) error {
	var unitType embedded.SystemdUnitType
	var unitsPath string
	switch ctx.PackageType {
	case PackageTypeDEB:
		unitType = embedded.SystemdUnitTypeDebRpm
		unitsPath = debUnitsPath
	case PackageTypeRPM:
		unitType = embedded.SystemdUnitTypeDebRpm
		unitsPath = rpmUnitsPath
	case PackageTypeOCI:
		unitType = embedded.SystemdUnitTypeOCI
		unitsPath = ociUnitsPath
	}
	for _, unit := range units {
		content, err := embedded.GetSystemdUnit(unit, unitType)
		if err != nil {
			return err
		}
		err = writeEmbeddedUnit(unitsPath, unit, content)
		if err != nil {
			return err
		}
	}
	return systemd.Reload(ctx)
}

func writeEmbeddedUnit(dir string, unit string, content []byte) error {
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}
	err = os.WriteFile(filepath.Join(dir, unit), content, 0644)
	if err != nil {
		return fmt.Errorf("failed to write file: %v", err)
	}
	return nil
}

func reverseStringSlice(slice []string) []string {
	reversed := make([]string, len(slice))
	copy(reversed, slice)
	slices.Reverse(reversed)
	return reversed
}
