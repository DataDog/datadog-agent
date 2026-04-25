// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package env

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// EnvInstallerRegistry is the canonical JSON env var the installer reads.
const EnvInstallerRegistry = "DD_INSTALLER_REGISTRY"

// Legacy per-package registry env var prefixes. Absorbed at the boundary
// (daemon + fxconfig bootstrap) into the unified DD_INSTALLER_REGISTRY
// JSON; not parsed directly by the installer.
const (
	envRegistryURLLegacy      = "DD_INSTALLER_REGISTRY_URL"
	envRegistryAuthLegacy     = "DD_INSTALLER_REGISTRY_AUTH"
	envRegistryUsernameLegacy = "DD_INSTALLER_REGISTRY_USERNAME"
	envRegistryPasswordLegacy = "DD_INSTALLER_REGISTRY_PASSWORD"
)

// LegacyRegistryEnvPrefixes is the list of per-package env var prefixes
// (e.g. DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE). Boundary translators
// consume and `os.Unsetenv` these after populating the unified Registry.
var LegacyRegistryEnvPrefixes = []string{
	envRegistryURLLegacy,
	envRegistryAuthLegacy,
	envRegistryUsernameLegacy,
	envRegistryPasswordLegacy,
}

// RegistryEntry is the set of overrides for a single registry.
type RegistryEntry struct {
	URL      string `json:"url,omitempty"`
	Auth     string `json:"auth,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// MergeStrings returns a copy of e with non-empty string args taking
// precedence over the corresponding field in e.
func (e RegistryEntry) MergeStrings(url, auth, username, password string) RegistryEntry {
	return e.merge(RegistryEntry{URL: url, Auth: auth, Username: username, Password: password})
}

// merge returns a copy of e with non-empty fields from o taking precedence.
func (e RegistryEntry) merge(o RegistryEntry) RegistryEntry {
	if o.URL != "" {
		e.URL = o.URL
	}
	if o.Auth != "" {
		e.Auth = o.Auth
	}
	if o.Username != "" {
		e.Username = o.Username
	}
	if o.Password != "" {
		e.Password = o.Password
	}
	return e
}

// PackageRegistry holds per-package registry overrides and per-extension
// sub-overrides for the package.
type PackageRegistry struct {
	RegistryEntry
	Extensions map[string]RegistryEntry `json:"extensions,omitempty"`
}

type RegistryConfig struct {
	Default  RegistryEntry              `json:"default,omitempty"`
	Packages map[string]PackageRegistry `json:"packages,omitempty"`
}

// Resolve returns the effective registry entry for a package and optional
// extension. Missing fields fall back to the package entry, then to Default.
// Empty pkg or unknown pkg returns Default. Empty ext returns the
// package-level entry.
func (r *RegistryConfig) Resolve(pkg, ext string) RegistryEntry {
	result := r.Default
	if pkg == "" {
		return result
	}
	pkgEntry, ok := r.Packages[pkg]
	if !ok {
		return result
	}
	result = result.merge(pkgEntry.RegistryEntry)
	if ext == "" {
		return result
	}
	if extEntry, ok := pkgEntry.Extensions[ext]; ok {
		result = result.merge(extEntry)
	}
	return result
}

// IsEmpty returns true if the RegistryConfig has no overrides at all.
func (r RegistryConfig) IsEmpty() bool {
	if r.Default != (RegistryEntry{}) {
		return false
	}
	for _, p := range r.Packages {
		if p.RegistryEntry != (RegistryEntry{}) || len(p.Extensions) > 0 {
			return false
		}
	}
	return true
}

// marshalRegistryIfNonEmpty returns the JSON encoding of r, or "" if r has
// no overrides. Used by ToEnv to omit the env var for empty configs.
func marshalRegistryIfNonEmpty(r RegistryConfig) string {
	if r.IsEmpty() {
		return ""
	}
	blob, err := json.Marshal(r)
	if err != nil {
		return ""
	}
	return string(blob)
}

// parseRegistryFromEnv reads DD_INSTALLER_REGISTRY from the process
// environment and returns the parsed RegistryConfig. An empty/unset var
// returns a zero RegistryConfig and no error. A set-but-invalid value
// returns an error.
func parseRegistryFromEnv() (RegistryConfig, error) {
	raw := os.Getenv(EnvInstallerRegistry)
	if raw == "" {
		return RegistryConfig{}, nil
	}
	var r RegistryConfig
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return RegistryConfig{}, fmt.Errorf("parsing %s: %w", EnvInstallerRegistry, err)
	}
	return r, nil
}

// packageNameFromEnvSuffix converts a legacy env var suffix back to a
// package name. E.g. "AGENT_PACKAGE" -> "datadog-agent". The inverse of
// the mapping used to build the env var name from the package name.
func packageNameFromEnvSuffix(suffix string) string {
	slug := strings.ToLower(strings.ReplaceAll(suffix, "_", "-"))
	slug = strings.TrimSuffix(slug, "-package")
	if slug == "" {
		return ""
	}
	return "datadog-" + slug
}

// PackageNameFromURL reverses oci.PackageURL's naming convention to derive
// the package name from a download URL. Returns "" for URLs that don't
// match the expected `<registry>/<slug>-package:<version>` shape.
func PackageNameFromURL(url string) string {
	slug := url[strings.LastIndex(url, "/")+1:]
	if i := strings.IndexAny(slug, ":@"); i >= 0 {
		slug = slug[:i]
	}
	if !strings.HasSuffix(slug, "-package") {
		return ""
	}
	slug = strings.TrimSuffix(slug, "-package")
	if slug == "" {
		return ""
	}
	return "datadog-" + slug
}

// applyLegacyEnvVars layers legacy DD_INSTALLER_REGISTRY_{URL,AUTH,
// USERNAME,PASSWORD}[_<PKG>] vars from the process env onto r, with
// per-field precedence: present legacy var overrides the matching field.
func (r *RegistryConfig) applyLegacyEnvVars() {
	// Global scalars → Default.
	if v := os.Getenv(envRegistryURLLegacy); v != "" {
		r.Default.URL = v
	}
	if v := os.Getenv(envRegistryAuthLegacy); v != "" {
		r.Default.Auth = v
	}
	if v := os.Getenv(envRegistryUsernameLegacy); v != "" {
		r.Default.Username = v
	}
	if v := os.Getenv(envRegistryPasswordLegacy); v != "" {
		r.Default.Password = v
	}

	// Per-package suffixed vars → Packages[pkg].
	// We iterate the process env once and dispatch on prefix.
	for _, kv := range os.Environ() {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			continue
		}
		key, val := kv[:eq], kv[eq+1:]
		if val == "" {
			continue
		}
		var (
			prefix string
			field  string
		)
		switch {
		case strings.HasPrefix(key, envRegistryURLLegacy+"_"):
			prefix, field = envRegistryURLLegacy+"_", "url"
		case strings.HasPrefix(key, envRegistryAuthLegacy+"_"):
			prefix, field = envRegistryAuthLegacy+"_", "auth"
		case strings.HasPrefix(key, envRegistryUsernameLegacy+"_"):
			prefix, field = envRegistryUsernameLegacy+"_", "username"
		case strings.HasPrefix(key, envRegistryPasswordLegacy+"_"):
			prefix, field = envRegistryPasswordLegacy+"_", "password"
		default:
			continue
		}
		suffix := strings.TrimPrefix(key, prefix)
		pkg := packageNameFromEnvSuffix(suffix)
		if pkg == "" {
			continue
		}
		if r.Packages == nil {
			r.Packages = map[string]PackageRegistry{}
		}
		entry := r.Packages[pkg]
		switch field {
		case "url":
			entry.URL = val
		case "auth":
			entry.Auth = val
		case "username":
			entry.Username = val
		case "password":
			entry.Password = val
		}
		r.Packages[pkg] = entry
	}
}

// applyRegistryEnvJSON layers any DD_INSTALLER_REGISTRY JSON value on top
// of r. JSON values override field-by-field; empty fields leave r's
// existing value in place.
func (r *RegistryConfig) applyRegistryEnvJSON() error {
	jsonReg, err := parseRegistryFromEnv()
	if err != nil {
		return err
	}
	r.merge(jsonReg)
	return nil
}

// merge layers o onto r in place. Scalar RegistryEntry fields overlay
// field-by-field; package entries are merged per package (and per
// extension).
func (r *RegistryConfig) merge(o RegistryConfig) {
	r.Default = r.Default.merge(o.Default)
	for name, pkg := range o.Packages {
		if r.Packages == nil {
			r.Packages = map[string]PackageRegistry{}
		}
		existing := r.Packages[name]
		existing.RegistryEntry = existing.RegistryEntry.merge(pkg.RegistryEntry)
		for ext, entry := range pkg.Extensions {
			if existing.Extensions == nil {
				existing.Extensions = map[string]RegistryEntry{}
			}
			existing.Extensions[ext] = existing.Extensions[ext].merge(entry)
		}
		r.Packages[name] = existing
	}
}

// ConfigReader is the minimal interface used by BuildRegistryFromConfigAndEnv
// to read installer-registry keys from datadog.yaml. Matches the subset of
// `comp/core/config.Component` we need, so we don't take a heavy dep here.
type ConfigReader interface {
	GetString(key string) string
	GetStringMap(key string) map[string]interface{}
}

// buildRegistryFromConfig populates a RegistryConfig from yaml
// `installer.registry.*` keys (incl. per-extension entries under
// `installer.registry.extensions.<pkg>.<ext>.*`).
func buildRegistryFromConfig(cfg ConfigReader) RegistryConfig {
	r := RegistryConfig{}
	r.Default = RegistryEntry{
		URL:      cfg.GetString("installer.registry.url"),
		Auth:     cfg.GetString("installer.registry.auth"),
		Username: cfg.GetString("installer.registry.username"),
		Password: cfg.GetString("installer.registry.password"),
	}
	exts := cfg.GetStringMap("installer.registry.extensions")
	for pkgName, extMapAny := range exts {
		extMap, ok := extMapAny.(map[string]interface{})
		if !ok {
			continue
		}
		pkg := PackageRegistry{}
		for extName, extCfgAny := range extMap {
			extCfg, ok := extCfgAny.(map[string]interface{})
			if !ok {
				continue
			}
			entry := RegistryEntry{}
			if s, ok := extCfg["url"].(string); ok {
				entry.URL = s
			}
			if s, ok := extCfg["auth"].(string); ok {
				entry.Auth = s
			}
			if s, ok := extCfg["username"].(string); ok {
				entry.Username = s
			}
			if s, ok := extCfg["password"].(string); ok {
				entry.Password = s
			}
			if entry == (RegistryEntry{}) {
				continue
			}
			if pkg.Extensions == nil {
				pkg.Extensions = map[string]RegistryEntry{}
			}
			pkg.Extensions[extName] = entry
		}
		if pkg.Extensions == nil && pkg.RegistryEntry == (RegistryEntry{}) {
			continue
		}
		if r.Packages == nil {
			r.Packages = map[string]PackageRegistry{}
		}
		r.Packages[pkgName] = pkg
	}
	return r
}

// BuildRegistryFromConfigAndEnv merges (in order, later wins on non-empty
// fields):
//  1. datadog.yaml `installer.registry.*` (incl. per-extension entries)
//  2. legacy DD_INSTALLER_REGISTRY_{URL,AUTH,USERNAME,PASSWORD}[_<PKG>]
//  3. DD_INSTALLER_REGISTRY JSON (if set)
//
// and returns the canonical RegistryConfig plus its JSON encoding. The
// caller is expected to emit the JSON as DD_INSTALLER_REGISTRY and unset
// the legacy vars so the installer sees a single, clean contract.
func BuildRegistryFromConfigAndEnv(cfg ConfigReader) (RegistryConfig, string, error) {
	r := buildRegistryFromConfig(cfg)
	r.applyLegacyEnvVars()
	if err := r.applyRegistryEnvJSON(); err != nil {
		return RegistryConfig{}, "", err
	}
	blob, err := json.Marshal(r)
	if err != nil {
		return RegistryConfig{}, "", err
	}
	return r, string(blob), nil
}

// BuildRegistryFromEnv is the no-config variant of
// BuildRegistryFromConfigAndEnv — used when only env vars are available
// (e.g. install-script entry points without an fx config.Component).
func BuildRegistryFromEnv() (RegistryConfig, string, error) {
	r := RegistryConfig{}
	r.applyLegacyEnvVars()
	if err := r.applyRegistryEnvJSON(); err != nil {
		return RegistryConfig{}, "", err
	}
	blob, err := json.Marshal(r)
	if err != nil {
		return RegistryConfig{}, "", err
	}
	return r, string(blob), nil
}
