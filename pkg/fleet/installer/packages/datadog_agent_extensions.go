// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
	// InfrastructureMode mirrors the top-level `infrastructure_mode` key in datadog.yaml
	// (full|basic|end_user_device|none). Used to gate End User Device Monitoring
	// extensions (e.g. eudm) when the mode is configured in datadog.yaml rather
	// than passed as DD_INFRASTRUCTURE_MODE at install time.
	InfrastructureMode string `yaml:"infrastructure_mode,omitempty"`
}

// infrastructureModeEndUserDevice is the infrastructure_mode value that enables
// End User Device Monitoring (EUDM).
//
//nolint:unused // Used in platform-specific files
const infrastructureModeEndUserDevice = "end_user_device"

//nolint:unused // Used in platform-specific files
type installerConfig struct {
	Registry installerRegistryConfig `yaml:"registry,omitempty"`
}

//nolint:unused // Used in platform-specific files
type extensionRegistryConfig struct {
	URL      string `yaml:"url,omitempty"`
	Auth     string `yaml:"auth,omitempty"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

//nolint:unused // Used in platform-specific files
type installerRegistryConfig struct {
	URL        string                                        `yaml:"url,omitempty"`
	Auth       string                                        `yaml:"auth,omitempty"`
	Username   string                                        `yaml:"username,omitempty"`
	Password   string                                        `yaml:"password,omitempty"`
	Extensions map[string]map[string]extensionRegistryConfig `yaml:"extensions,omitempty"`
}

// setRegistryConfig is a best effort to get the `installer` block from `datadog.yaml` and update the env.
// It returns per-extension registry overrides parsed from installer.registry.extensions.<pkg>.<ext>.
//
//nolint:unused // Used in platform-specific files
func setRegistryConfig(env *env.Env) map[string]extensionsPkg.ExtensionRegistry {
	config, ok := loadDatadogAgentConfig()
	if !ok {
		return nil
	}

	// Update env with values from config if not already set.
	// DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE takes precedence over installer.registry.url.
	if env.RegistryOverride == "" {
		if agentPackageURL := os.Getenv("DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE"); agentPackageURL != "" {
			env.RegistryOverride = agentPackageURL
		} else if config.Installer.Registry.URL != "" {
			env.RegistryOverride = config.Installer.Registry.URL
		}
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

	// Parse per-extension registry overrides for the agent package.
	extConfigs := config.Installer.Registry.Extensions[agentPackage]
	if len(extConfigs) == 0 {
		return nil
	}
	overrides := make(map[string]extensionsPkg.ExtensionRegistry, len(extConfigs))
	for extName, extCfg := range extConfigs {
		overrides[extName] = extensionsPkg.ExtensionRegistry{
			URL:      extCfg.URL,
			Auth:     extCfg.Auth,
			Username: extCfg.Username,
			Password: extCfg.Password,
		}
	}
	return overrides
}

// isEndUserDeviceMode reports whether End User Device Monitoring (EUDM) is enabled.
//
// DD_INFRASTRUCTURE_MODE is authoritative when set: any explicit value that is not
// end_user_device disables EUDM, even if datadog.yaml still says otherwise. The
// infrastructure_mode value from datadog.yaml is only used as a fallback when the env var is
// blank (e.g. upgrades that do not re-supply DD_INFRASTRUCTURE_MODE).
//
//nolint:unused // Used in platform-specific files
func isEndUserDeviceMode(env *env.Env) bool {
	return endUserDeviceModeEnabled(env.InfrastructureMode, readInfrastructureModeFromConfig)
}

// endUserDeviceModeEnabled applies the EUDM gating precedence. envMode (DD_INFRASTRUCTURE_MODE) is
// authoritative when non-empty; configModeFn (the datadog.yaml value) is consulted only as a
// fallback when envMode is blank, and is not called otherwise.
//
//nolint:unused // Used in platform-specific files
func endUserDeviceModeEnabled(envMode string, configModeFn func() string) bool {
	if envMode != "" {
		return strings.EqualFold(envMode, infrastructureModeEndUserDevice)
	}
	return strings.EqualFold(configModeFn(), infrastructureModeEndUserDevice)
}

// readInfrastructureModeFromConfig returns the infrastructure_mode value from datadog.yaml,
// or "" if the file cannot be read/parsed or the key is unset.
//
//nolint:unused // Used in platform-specific files
func readInfrastructureModeFromConfig() string {
	config, ok := loadDatadogAgentConfig()
	if !ok {
		return ""
	}
	return config.InfrastructureMode
}

// loadDatadogAgentConfig reads and parses the subset of the installed datadog.yaml that the
// installer cares about. It returns ok=false (best effort) if the file cannot be read or parsed.
//
//nolint:unused // Used in platform-specific files
func loadDatadogAgentConfig() (datadogAgentConfig, bool) {
	configPath := filepath.Join(paths.AgentConfigDir, "datadog.yaml")
	rawConfig, err := os.ReadFile(configPath)
	if err != nil {
		log.Debugf("could not read agent config at %s: %v", configPath, err)
		return datadogAgentConfig{}, false
	}
	var config datadogAgentConfig
	if err := yaml.Unmarshal(rawConfig, &config); err != nil {
		log.Warnf("could not parse agent config at %s: %v", configPath, err)
		return datadogAgentConfig{}, false
	}
	return config, true
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
	overrides := setRegistryConfig(env)

	downloader := oci.NewDownloader(env, env.HTTPClient())
	url := oci.PackageURL(env, agentPackage, version)
	hooks := NewHooks(env, repository.NewRepositories(paths.PackagesPath, AsyncPreRemoveHooks))

	// Restore replays whatever extensions were previously installed. Once an extension is
	// installed it stays installed across upgrades; enabling conditions (e.g. EUDM) only gate the
	// initial install in installAgentExtensions, not restore.
	return extensionsPkg.Restore(ctx, downloader, agentPackage, url, storagePath, experiment, hooks, overrides)
}

// installAgentExtensions installs the given extensions for the agent package.
// Extensions that are already installed (e.g. from a prior restore) are skipped
// by the idempotency check in extensionsPkg.Install.
//
//nolint:unused // Used in platform-specific files
func installAgentExtensions(ctx HookContext, version string, isExperiment bool) error {
	env := env.FromEnv()
	// populate extensions list based on environment variables
	var extensions []string
	if env.OTelCollectorEnabled {
		extensions = append(extensions, "ddot")
	}
	// The eudm extension (currently the AI Usage Chrome Native Messaging host + desktop monitor;
	// a container for End User Device Monitoring features) is Windows-only and gated on EUDM. It
	// is enabled when DD_INFRASTRUCTURE_MODE=end_user_device is passed at install time; when that
	// env var is blank it falls back to infrastructure_mode: end_user_device in datadog.yaml
	// (covers upgrades that do not re-supply the env var). See isEndUserDeviceMode.
	if runtime.GOOS == "windows" && isEndUserDeviceMode(env) {
		extensions = append(extensions, "eudm")
	}
	// if no extensions are requested, return early
	if len(extensions) == 0 {
		return nil
	}

	// In testing environments the binary's compiled-in version (e.g. 7.79.0-devel-1)
	// differs from the pipeline OCI tag (e.g. pipeline-107898846). Allow the caller to
	// override via DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_AGENT so the correct image
	// is fetched without modifying the installed binary.
	if override := env.DefaultPackagesVersionOverride[agentPackage]; override != "" {
		version = override
	}

	// install extensions
	overrides := setRegistryConfig(env)
	downloader := oci.NewDownloader(env, env.HTTPClient())
	url := oci.PackageURL(env, agentPackage, version)
	hooks := NewHooks(env, repository.NewRepositories(paths.PackagesPath, AsyncPreRemoveHooks))
	return extensionsPkg.Install(ctx, downloader, url, extensions, isExperiment, hooks, overrides)
}
