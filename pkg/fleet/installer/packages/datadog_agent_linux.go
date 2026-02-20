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
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/installinfo"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
	extensionsPkg "github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/extensions"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/fapolicyd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/file"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/integrations"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/packagemanager"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/selinux"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/systemd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/sysvinit"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/upstart"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/user"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
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

	preInstallExtension:  preInstallExtensionDatadogAgent,
	postInstallExtension: postInstallExtensionDatadogAgent,
	preRemoveExtension:   preRemoveExtensionDatadogAgent,
}

const (
	agentPackage     = "datadog-agent"
	agentSymlink     = "/usr/bin/datadog-agent"
	installerSymlink = "/usr/bin/datadog-installer"
)

var (
	// agentDirectories are the directories that the agent needs to function
	agentDirectories = file.Directories{
		{Path: "/etc/datadog-agent", Mode: 0755, Owner: "dd-agent", Group: "dd-agent"},
		{Path: "/etc/datadog-agent/managed", Mode: 0755, Owner: "dd-agent", Group: "dd-agent"},
		{Path: "/var/log/datadog", Mode: 0750, Owner: "dd-agent", Group: "dd-agent"},
		{Path: "/opt/datadog-packages/run", Mode: 0755, Owner: "dd-agent", Group: "dd-agent"},
		{Path: "/opt/datadog-packages/tmp", Mode: 0755, Owner: "dd-agent", Group: "dd-agent"},
	}

	// agentConfigPermissions are the ownerships and modes that are enforced on the agent configuration files
	agentConfigPermissions = file.Permissions{
		{Path: ".", Owner: "dd-agent", Group: "dd-agent", Recursive: true},
		{Path: "managed", Owner: "dd-agent", Group: "dd-agent", Recursive: true},
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

	// agentPackageUninstallPaths are the agent paths that are deleted during an uninstall
	agentPackageUninstallPaths = file.Paths{
		"embedded/ssl/fipsmodule.cnf",
		"run",
		".pre_python_installed_packages.txt",
		".post_python_installed_packages.txt",
		".diff_python_installed_packages.txt",
	}

	// installerPackageUninstallPaths are the installer paths that are deleted during an uninstall
	// The only one left is packages.db, which is owned by root and will cause no issue during reinstallation.
	installerPackageUninstallPaths = file.Paths{
		"run", // Includes RC DB & Task DB
		"tmp",
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
		SystemdUnitsStable:    []string{"datadog-agent.service", "datadog-agent-installer.service", "datadog-agent-trace.service", "datadog-agent-process.service", "datadog-agent-sysprobe.service", "datadog-agent-security.service", "datadog-agent-data-plane.service", "datadog-agent-action.service", "datadog-agent-ddot.service", "datadog-agent-procmgrd.service"},
		SystemdUnitsExp:       []string{"datadog-agent-exp.service", "datadog-agent-installer-exp.service", "datadog-agent-trace-exp.service", "datadog-agent-process-exp.service", "datadog-agent-sysprobe-exp.service", "datadog-agent-security-exp.service", "datadog-agent-data-plane-exp.service", "datadog-agent-action-exp.service", "datadog-agent-ddot-exp.service", "datadog-agent-procmgrd-exp.service"},

		UpstartMainService: "datadog-agent",
		UpstartServices:    []string{"datadog-agent", "datadog-agent-trace", "datadog-agent-process", "datadog-agent-sysprobe", "datadog-agent-security", "datadog-agent-data-plane", "datadog-agent-action"},

		SysvinitMainService: "datadog-agent",
		SysvinitServices:    []string{"datadog-agent", "datadog-agent-trace", "datadog-agent-process", "datadog-agent-security", "datadog-agent-data-plane", "datadog-agent-action"},
	}

	// oldInstallerUnitsPaths are the deb/rpm/oci installer package unit paths
	oldInstallerUnitPaths = file.Paths{
		"datadog-installer-exp.service",
		"datadog-installer.service",
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

	// 2. Ensure config/run/log/package directories are created and have the correct permissions
	if err = agentDirectories.Ensure(ctx); err != nil {
		return fmt.Errorf("failed to create directories: %v", err)
	}
	if err = agentPackagePermissions.Ensure(ctx, ctx.PackagePath); err != nil {
		return fmt.Errorf("failed to set package ownerships: %v", err)
	}
	if err = agentConfigPermissions.Ensure(ctx, "/etc/datadog-agent"); err != nil {
		return fmt.Errorf("failed to set config ownerships: %v", err)
	}
	agentRunPath := file.Directory{Path: filepath.Join(ctx.PackagePath, "run"), Mode: 0755, Owner: "dd-agent", Group: "dd-agent"}
	if err = agentRunPath.Ensure(ctx); err != nil {
		return fmt.Errorf("failed to create run directory: %v", err)
	}

	// 3. Create symlinks
	if err = file.EnsureSymlink(ctx, filepath.Join(ctx.PackagePath, "bin/agent/agent"), agentSymlink); err != nil {
		return fmt.Errorf("failed to create symlink: %v", err)
	}
	if err = file.EnsureSymlink(ctx, filepath.Join(ctx.PackagePath, "embedded/bin/installer"), installerSymlink); err != nil {
		return fmt.Errorf("failed to create symlink: %v", err)
	}

	// 4. Set up SELinux permissions
	if err = selinux.SetAgentPermissions(ctx, "/etc/datadog-agent", ctx.PackagePath); err != nil {
		log.Warnf("failed to set SELinux permissions: %v", err)
	}

	// 5. Handle install info
	if err = installinfo.WriteInstallInfo(ctx, string(ctx.PackageType)); err != nil {
		return fmt.Errorf("failed to write install info: %v", err)
	}

	// 6. Remove old installer units if they exist
	if err = oldInstallerUnitPaths.EnsureAbsent(ctx, "/etc/systemd/system"); err != nil {
		return fmt.Errorf("failed to remove old installer units: %v", err)
	}
	return nil
}

// uninstallFilesystem cleans the filesystem by removing various temporary files, symlinks and installation metadata
func uninstallFilesystem(ctx HookContext) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "remove_filesystem")
	defer func() {
		span.Finish(err)
	}()

	err = agentPackageUninstallPaths.EnsureAbsent(ctx, ctx.PackagePath)
	if err != nil {
		return fmt.Errorf("failed to remove package paths: %w", err)
	}
	err = installerPackageUninstallPaths.EnsureAbsent(ctx, paths.PackagesPath)
	if err != nil {
		return fmt.Errorf("failed to remove installer package paths: %w", err)
	}
	err = agentConfigUninstallPaths.EnsureAbsent(ctx, "/etc/datadog-agent")
	if err != nil {
		return fmt.Errorf("failed to remove config paths: %w", err)
	}
	err = file.EnsureSymlinkAbsent(ctx, agentSymlink)
	if err != nil {
		return fmt.Errorf("failed to remove agent symlink: %w", err)
	}

	installerTarget, err := os.Readlink(installerSymlink)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read installer symlink: %w", err)
	}
	if err == nil && strings.HasPrefix(installerTarget, ctx.PackagePath) {
		err = file.EnsureSymlinkAbsent(ctx, installerSymlink)
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
	if ctx.PackageType == PackageTypeOCI {
		// Must be called in the OCI preinst, before re-executing into the installer
		if err := fapolicyd.SetAgentPermissions(ctx); err != nil {
			return fmt.Errorf("failed to ensure host security context: %w", err)
		}
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
	if err := extensionsPkg.SetPackage(ctx, agentPackage, getCurrentAgentVersion(), false); err != nil {
		return fmt.Errorf("failed to set package version in extensions db: %w", err)
	}
	if err := restoreAgentExtensions(ctx, false); err != nil {
		fmt.Printf("failed to restore extensions: %s\n", err.Error())
		log.Warnf("failed to restore extensions: %s", err)
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
		if err := removeAgentExtensions(ctx, false); err != nil {
			log.Warnf("failed to remove agent extensions: %s", err)
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
		if err := saveAgentExtensions(ctx); err != nil {
			log.Warnf("failed to save agent extensions: %s", err)
		}
		if err := removeAgentExtensions(ctx, false); err != nil {
			log.Warnf("failed to remove agent extensions: %s", err)
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
	if err := saveAgentExtensions(ctx); err != nil {
		log.Warnf("failed to save agent extensions: %s", err)
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
	if err := restoreAgentExtensions(ctx, true); err != nil {
		log.Warnf("failed to restore agent extensions: %s", err)
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
	if err := extensionsPkg.DeletePackage(ctx, agentPackage, true); err != nil {
		return fmt.Errorf("failed to delete agent extensions: %s", err)
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
	err = extensionsPkg.Promote(ctx, agentPackage)
	if err != nil {
		return fmt.Errorf("failed to promote extensions: %s", err)
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

type datadogAgentConfig struct {
	Installer installerConfig `yaml:"installer"`
}

type installerConfig struct {
	Registry installerRegistryConfig `yaml:"registry,omitempty"`
}

type installerRegistryConfig struct {
	URL      string `yaml:"url,omitempty"`
	Auth     string `yaml:"auth,omitempty"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

// preInstallExtensionDatadogAgent runs pre-installation steps for agent extensions
func preInstallExtensionDatadogAgent(ctx HookContext) error {
	switch ctx.Extension {
	case "ddot":
		return preInstallDDOTExtension(ctx)
	default:
		return nil
	}
}

// postInstallExtensionDatadogAgent runs post-installation steps for agent extensions
func postInstallExtensionDatadogAgent(ctx HookContext) error {
	extensionPath := filepath.Join(ctx.PackagePath, "ext", ctx.Extension)

	// Set ownership recursively to dd-agent:dd-agent for all extensions
	extensionPermissions := file.Permissions{
		{Path: ".", Owner: "dd-agent", Group: "dd-agent", Recursive: true},
	}
	if err := extensionPermissions.Ensure(ctx, extensionPath); err != nil {
		return fmt.Errorf("failed to set extension ownerships: %v", err)
	}

	switch ctx.Extension {
	case "ddot":
		return postInstallDDOTExtension(ctx)
	default:
		return nil
	}
}

// preRemoveExtensionDatadogAgent runs pre-removal steps for agent extensions
func preRemoveExtensionDatadogAgent(ctx HookContext) error {
	switch ctx.Extension {
	case "ddot":
		return preRemoveDDOTExtension(ctx)
	default:
		return nil
	}
}

// setRegistryConfig is a best effort to get the `installer` block from `datadog.yaml` and update the env.
func setRegistryConfig(env *env.Env) {
	configPath := filepath.Join(paths.AgentConfigDir, "datadog.yaml")
	rawConfig, err := os.ReadFile(configPath)
	if err != nil {
		return
	}
	var config datadogAgentConfig
	err = yaml.Unmarshal(rawConfig, &config)
	if err != nil {
		return
	}

	// Update env with values from config if not already set
	if config.Installer.Registry.URL != "" && env.RegistryOverride == "" {
		env.RegistryOverride = config.Installer.Registry.URL
	}
	if config.Installer.Registry.Auth != "" && env.RegistryAuthOverride == "" {
		env.RegistryAuthOverride = config.Installer.Registry.Auth
	}
	if config.Installer.Registry.Username != "" && env.RegistryUsername == "" {
		env.RegistryUsername = config.Installer.Registry.Username
	}
	if config.Installer.Registry.Password != "" && env.RegistryPassword == "" {
		env.RegistryPassword = config.Installer.Registry.Password
	}
}

// saveAgentExtensions saves the extensions of the Agent package by writing them to a file on disk.
// the extensions can then be picked up by the restoreAgentExtensions function to restore them
func saveAgentExtensions(ctx HookContext) error {
	storagePath := ctx.PackagePath
	if strings.HasPrefix(ctx.PackagePath, paths.PackagesPath) {
		storagePath = paths.RootTmpDir
	}

	return extensionsPkg.Save(ctx, agentPackage, storagePath)
}

// removeAgentExtensions removes the extensions of the Agent package & then deletes the package from the extensions db.
func removeAgentExtensions(ctx HookContext, experiment bool) error {
	env := env.FromEnv()
	hooks := NewHooks(env, repository.NewRepositories(paths.PackagesPath, AsyncPreRemoveHooks))
	err := extensionsPkg.RemoveAll(ctx, agentPackage, experiment, hooks)
	if err != nil {
		return fmt.Errorf("failed to remove all extensions: %w", err)
	}
	return extensionsPkg.DeletePackage(ctx, agentPackage, experiment)
}

// restoreAgentExtensions restores the extensions for a package by setting the new package version in the extensions db &
// then reading the extensions from a file on disk
func restoreAgentExtensions(ctx HookContext, experiment bool) error {
	if err := extensionsPkg.SetPackage(ctx, agentPackage, getCurrentAgentVersion(), experiment); err != nil {
		return fmt.Errorf("failed to set package version in extensions db: %w", err)
	}

	storagePath := ctx.PackagePath
	if strings.HasPrefix(ctx.PackagePath, paths.PackagesPath) {
		storagePath = paths.RootTmpDir
	}

	env := env.FromEnv()

	// Best effort to get the registry config from datadog.yaml
	setRegistryConfig(env)

	downloader := oci.NewDownloader(env, env.HTTPClient())
	url := oci.PackageURL(env, agentPackage, getCurrentAgentVersion())
	hooks := NewHooks(env, repository.NewRepositories(paths.PackagesPath, AsyncPreRemoveHooks))

	return extensionsPkg.Restore(ctx, downloader, agentPackage, url, storagePath, experiment, hooks)
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
			return errors.New("upstart is only supported in DEB and RPM packages")
		}
	case service.SysvinitType:
		if ctx.PackageType != PackageTypeDEB {
			return errors.New("sysvinit is only supported in DEB packages")
		}
	default:
		return errors.New("could not determine service manager type, platform is not supported")
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
		return errors.New("unsupported service manager")
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
		return errors.New("unsupported service manager")
	}
}

// RestartStable restarts the stable unit. It will only attempt to restart if the config exists.
// The systemd unit will be reset first to avoid triggering the restart limit.
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
		return errors.New("unsupported service manager")
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
		return errors.New("unsupported service manager")
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
	return errors.New("unsupported service manager")
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
	return errors.New("unsupported service manager")
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
		return errors.New("experiments are not supported on upstart")
	case service.SysvinitType:
		return errors.New("experiments are not supported on sysvinit")
	}
	return errors.New("unsupported service manager")
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
	return errors.New("unsupported service manager")
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
		return errors.New("experiments are not supported on upstart")
	case service.SysvinitType:
		return errors.New("experiments are not supported on sysvinit")
	}
	return errors.New("unsupported service manager")
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
	return errors.New("unsupported service manager")
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
	ambiantCapabilitiesSupported, err := isAmbiantCapabilitiesSupported()
	if err != nil {
		log.Errorf("failed to check if ambiant capabilities are supported: %v", err)
		ambiantCapabilitiesSupported = true // Assume true if we can't check
	}
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
		content, err := embedded.GetSystemdUnit(unit, unitType, ambiantCapabilitiesSupported)
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

func isAmbiantCapabilitiesSupported() (bool, error) {
	content, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return false, fmt.Errorf("failed to read /proc/self/status: %v", err)
	}
	return strings.Contains(string(content), "CapAmb:"), nil
}

func getCurrentAgentVersion() string {
	v := version.AgentVersionURLSafe
	if strings.HasSuffix(v, "-1") {
		return v
	}
	return v + "-1"
}

// RestartDatadogAgent restarts the datadog-agent service if it is running
func RestartDatadogAgent(ctx context.Context) error {
	if ok, err := systemd.IsRunning(); err != nil || !ok {
		return nil
	}
	return systemd.RestartUnit(ctx, "datadog-agent.service")
}
