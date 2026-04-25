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

	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
	extensionsPkg "github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/extensions"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// installerYAMLSchema is a minimal subset of datadog.yaml used as a
// last-resort fallback when the env-var boundary translation didn't fire.
//
// Context: on Windows MSI deferred custom actions, the installer is
// spawned by RunPostInstallHook with a stripped environment — the CLI
// fxconfig bootstrap should still read datadog.yaml and populate
// DD_INSTALLER_REGISTRY, but observed behavior in CI shows the
// registry override sometimes doesn't reach restoreAgentExtensions.
// This fallback reads yaml directly in the hook so MSI upgrades of
// agents that have an `installer.registry.*` block continue to restore
// extensions from the correct registry.
//
//nolint:unused // Used in platform-specific files
type installerYAMLSchema struct {
	Installer struct {
		Registry struct {
			URL        string                                   `yaml:"url,omitempty"`
			Auth       string                                   `yaml:"auth,omitempty"`
			Username   string                                   `yaml:"username,omitempty"`
			Password   string                                   `yaml:"password,omitempty"`
			Extensions map[string]map[string]yamlExtensionEntry `yaml:"extensions,omitempty"`
		} `yaml:"registry,omitempty"`
	} `yaml:"installer,omitempty"`
}

//nolint:unused // Used in platform-specific files
type yamlExtensionEntry struct {
	URL      string `yaml:"url,omitempty"`
	Auth     string `yaml:"auth,omitempty"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

// applyYAMLRegistryFallback reads datadog.yaml and populates env.Registry
// for any fields the boundary translation didn't set. This is specifically
// a safety net for hooks running on Windows MSI where the env may be
// stripped — see the `installerYAMLSchema` doc for context.
//
//nolint:unused // Used in platform-specific files
func applyYAMLRegistryFallback(e *env.Env) {
	configPath := filepath.Join(paths.AgentConfigDir, "datadog.yaml")
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return
	}
	var cfg installerYAMLSchema
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		log.Debugf("fallback: could not parse %s: %v", configPath, err)
		return
	}
	reg := cfg.Installer.Registry
	// Layer yaml values into Default — only fields that aren't already set
	// by the env boundary (so env > yaml precedence holds).
	if reg.URL != "" && e.Registry.Default.URL == "" {
		e.Registry.Default.URL = reg.URL
	}
	if reg.Auth != "" && e.Registry.Default.Auth == "" {
		e.Registry.Default.Auth = reg.Auth
	}
	if reg.Username != "" && e.Registry.Default.Username == "" {
		e.Registry.Default.Username = reg.Username
	}
	if reg.Password != "" && e.Registry.Default.Password == "" {
		e.Registry.Default.Password = reg.Password
	}
	// Per-extension entries under the agent package.
	agentExts := reg.Extensions[agentPackage]
	if len(agentExts) == 0 {
		return
	}
	if e.Registry.Packages == nil {
		e.Registry.Packages = map[string]env.PackageRegistry{}
	}
	pkg := e.Registry.Packages[agentPackage]
	if pkg.Extensions == nil {
		pkg.Extensions = map[string]env.RegistryEntry{}
	}
	for name, entry := range agentExts {
		existing, ok := pkg.Extensions[name]
		if ok && existing != (env.RegistryEntry{}) {
			continue // don't overwrite env-sourced entries
		}
		pkg.Extensions[name] = env.RegistryEntry{
			URL:      entry.URL,
			Auth:     entry.Auth,
			Username: entry.Username,
			Password: entry.Password,
		}
	}
	e.Registry.Packages[agentPackage] = pkg
}

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

// extensionOverrides returns per-extension registry override entries for
// the agent package, resolved from the unified env.Registry.
//
// If `extensions` is non-nil, only the named ones are returned (used at
// install time when the caller already knows what's being installed). If
// `extensions` is nil, ALL extensions defined under the package are
// returned — needed by restoreAgentExtensions, which discovers extensions
// from disk and has to pass the full override map downstream.
//
//nolint:unused // Used in platform-specific files
func extensionOverrides(e *env.Env, extensions []string) map[string]extensionsPkg.ExtensionRegistry {
	pkgEntry, hasPkg := e.Registry.Packages[agentPackage]
	if !hasPkg || len(pkgEntry.Extensions) == 0 {
		return nil
	}
	wanted := func(name string) bool {
		if extensions == nil {
			return true
		}
		for _, ext := range extensions {
			if ext == name {
				return true
			}
		}
		return false
	}
	overrides := make(map[string]extensionsPkg.ExtensionRegistry)
	for name, entry := range pkgEntry.Extensions {
		if !wanted(name) {
			continue
		}
		overrides[name] = extensionsPkg.ExtensionRegistry{
			URL:      entry.URL,
			Auth:     entry.Auth,
			Username: entry.Username,
			Password: entry.Password,
		}
	}
	if len(overrides) == 0 {
		return nil
	}
	return overrides
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
	applyYAMLRegistryFallback(env)

	storagePath := getExtensionStoragePath(ctx.PackagePath)

	overrides := extensionOverrides(env, nil)

	downloader := oci.NewDownloader(env, env.HTTPClient())
	url := oci.PackageURL(env, agentPackage, version)
	hooks := NewHooks(env, repository.NewRepositories(paths.PackagesPath, AsyncPreRemoveHooks))

	return extensionsPkg.Restore(ctx, downloader, agentPackage, url, storagePath, experiment, hooks, overrides)
}

// installAgentExtensions installs the given extensions for the agent package.
// Extensions that are already installed (e.g. from a prior restore) are skipped
// by the idempotency check in extensionsPkg.Install.
//
//nolint:unused // Used in platform-specific files
func installAgentExtensions(ctx HookContext, version string, isExperiment bool) error {
	env := env.FromEnv()
	applyYAMLRegistryFallback(env)
	// populate extensions list based on environment variables
	var extensions []string
	if env.OTelCollectorEnabled {
		extensions = append(extensions, "ddot")
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
	overrides := extensionOverrides(env, extensions)
	downloader := oci.NewDownloader(env, env.HTTPClient())
	url := oci.PackageURL(env, agentPackage, version)
	hooks := NewHooks(env, repository.NewRepositories(paths.PackagesPath, AsyncPreRemoveHooks))
	return extensionsPkg.Install(ctx, downloader, url, extensions, isExperiment, hooks, overrides)
}
