// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
	extensionsPkg "github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/extensions"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
)

const agentPackageName = "datadog-agent"

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

// setRegistryConfig is a best effort to get the `installer` block from `datadog.yaml` and update the env.
func setRegistryConfig(e *env.Env) {
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

	if config.Installer.Registry.URL != "" && e.RegistryOverride == "" {
		e.RegistryOverride = config.Installer.Registry.URL
	}
	if config.Installer.Registry.Auth != "" && e.RegistryAuthOverride == "" {
		e.RegistryAuthOverride = config.Installer.Registry.Auth
	}
	if config.Installer.Registry.Username != "" && e.RegistryUsername == "" {
		e.RegistryUsername = config.Installer.Registry.Username
	}
	if config.Installer.Registry.Password != "" && e.RegistryPassword == "" {
		e.RegistryPassword = config.Installer.Registry.Password
	}
}

// saveAgentExtensions saves the extensions of the Agent package by writing them to a file on disk.
// The extensions can then be picked up by the restoreAgentExtensions function to restore them.
func saveAgentExtensions(ctx HookContext) error {
	storagePath := ctx.PackagePath
	if strings.HasPrefix(ctx.PackagePath, paths.PackagesPath) {
		storagePath = paths.RootTmpDir
	}
	return extensionsPkg.Save(ctx, agentPackageName, storagePath)
}

// removeAgentExtensions removes the extensions of the Agent package and then deletes the package from the extensions db.
func removeAgentExtensions(ctx HookContext, experiment bool) error {
	e := env.FromEnv()
	hooks := NewHooks(e, repository.NewRepositories(paths.PackagesPath, AsyncPreRemoveHooks))
	err := extensionsPkg.RemoveAll(ctx, agentPackageName, experiment, hooks)
	if err != nil {
		return fmt.Errorf("failed to remove all extensions: %w", err)
	}
	return extensionsPkg.DeletePackage(ctx, agentPackageName, experiment)
}

// restoreAgentExtensions restores the extensions for a package by setting the new package version in the extensions db
// and then reading the extensions from a file on disk.
// agentVersion is the version string to use for the package (format differs by platform).
func restoreAgentExtensions(ctx HookContext, agentVersion string, experiment bool) error {
	if err := extensionsPkg.SetPackage(ctx, agentPackageName, agentVersion, experiment); err != nil {
		return fmt.Errorf("failed to set package version in extensions db: %w", err)
	}

	storagePath := ctx.PackagePath
	if strings.HasPrefix(ctx.PackagePath, paths.PackagesPath) {
		storagePath = paths.RootTmpDir
	}

	e := env.FromEnv()
	setRegistryConfig(e)

	downloader := oci.NewDownloader(e, e.HTTPClient())
	url := oci.PackageURL(e, agentPackageName, agentVersion)
	// url = "file:///agent-package" // TODO: for testing
	hooks := NewHooks(e, repository.NewRepositories(paths.PackagesPath, AsyncPreRemoveHooks))

	return extensionsPkg.Restore(ctx, downloader, agentPackageName, url, storagePath, experiment, hooks)
}
