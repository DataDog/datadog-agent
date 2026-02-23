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
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

//nolint:unused // Used in platform-specific files
const agentPackage = "datadog-agent"

// getCurrentAgentVersion returns the current agent version in URL-safe format with -1 suffix
//
//nolint:unused // Used in platform-specific files
func getCurrentAgentVersion() string {
	v := version.AgentVersionURLSafe
	if strings.HasSuffix(v, "-1") {
		return v
	}
	return v + "-1"
}

// Config structs for reading installer registry configuration from datadog.yaml

//nolint:unused // Used in platform-specific files
type datadogAgentConfig struct {
	Installer installerConfig `yaml:"installer"`
}

//nolint:unused // Used in platform-specific files
type installerConfig struct {
	Registry installerRegistryConfig `yaml:"registry,omitempty"`
}

//nolint:unused // Used in platform-specific files
type installerRegistryConfig struct {
	URL      string `yaml:"url,omitempty"`
	Auth     string `yaml:"auth,omitempty"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

// setRegistryConfig is a best effort to get the `installer` block from `datadog.yaml` and update the env.
//
//nolint:unused // Used in platform-specific files
func setRegistryConfig(env *env.Env) {
	configPath := filepath.Join(paths.AgentConfigDir, "datadog.yaml")
	rawConfig, err := os.ReadFile(configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Debugf("could not read agent config at %s: %v", configPath, err)
		}
		return
	}
	var config datadogAgentConfig
	err = yaml.Unmarshal(rawConfig, &config)
	if err != nil {
		log.Warnf("could not parse agent config at %s: %v", configPath, err)
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
//
//nolint:unused // Used in platform-specific files
func saveAgentExtensions(ctx HookContext, isExperiment bool) error {
	storagePath := getExtensionStoragePath(ctx.PackagePath)
	return extensionsPkg.Save(ctx, agentPackage, storagePath, isExperiment)
}

// removeAgentExtensions removes the extensions of the Agent package & then deletes the package from the extensions db.
//
//nolint:unused // Used in platform-specific files
func removeAgentExtensions(ctx HookContext, experiment bool) error {
	env := env.FromEnv()
	hooks := NewHooks(env, repository.NewRepositories(paths.PackagesPath, AsyncPreRemoveHooks))
	err := extensionsPkg.RemoveAll(ctx, agentPackage, experiment, hooks)
	if err != nil {
		return fmt.Errorf("failed to remove all extensions: %w", err)
	}
	return extensionsPkg.DeletePackage(ctx, agentPackage, experiment)
}

// restoreAgentExtensions restores the extensions for a package by reading the extensions from a file on disk.
// Note: Caller must call extensionsPkg.SetPackage() separately before calling this function.
//
//nolint:unused // Used in platform-specific files
func restoreAgentExtensions(ctx HookContext, version string, experiment bool) error {
	env := env.FromEnv()

	storagePath := getExtensionStoragePath(ctx.PackagePath)

	// Best effort to get the registry config from datadog.yaml
	setRegistryConfig(env)

	downloader := oci.NewDownloader(env, env.HTTPClient())
	url := oci.PackageURL(env, agentPackage, version)
	hooks := NewHooks(env, repository.NewRepositories(paths.PackagesPath, AsyncPreRemoveHooks))

	return extensionsPkg.Restore(ctx, downloader, agentPackage, url, storagePath, experiment, hooks)
}
