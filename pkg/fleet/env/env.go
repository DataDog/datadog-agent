// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package env provides the environment variables for the installer.
package env

import (
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

const (
	envAPIKey         = "DD_API_KEY"
	envSite           = "DD_SITE"
	envRegistry       = "DD_INSTALLER_REGISTRY"
	envRegistryAuth   = "DD_INSTALLER_REGISTRY_AUTH"
	envDefaultVersion = "DD_INSTALLER_DEFAULT_VERSION"
)

var defaultEnv = Env{
	APIKey: "",
	Site:   "datadoghq.com",

	RegistryOverride:                "",
	RegistryAuthOverride:            []string{},
	RegistryOverrideByPackage:       map[string]string{},
	DefaultVersionOverrideByPackage: map[string]string{},
}

// Env contains the configuration for the installer.
type Env struct {
	APIKey string
	Site   string

	RegistryOverride                string
	RegistryAuthOverride            []string
	RegistryOverrideByPackage       map[string]string
	DefaultVersionOverrideByPackage map[string]string
}

// FromEnv returns an Env struct with values from the environment.
func FromEnv() *Env {
	return &Env{
		APIKey: getEnvOrDefault(envAPIKey, defaultEnv.APIKey),
		Site:   getEnvOrDefault(envSite, defaultEnv.Site),

		RegistryOverride:                getEnvOrDefault(envRegistry, defaultEnv.RegistryOverride),
		RegistryAuthOverride:            authOverridesFromString(os.Getenv(envRegistryAuth)),
		RegistryOverrideByPackage:       overridesByPackageFromEnv(envRegistry),
		DefaultVersionOverrideByPackage: overridesByPackageFromEnv(envDefaultVersion),
	}
}

// FromConfig returns an Env struct with values from the configuration.
func FromConfig(config config.Reader) *Env {
	return &Env{
		APIKey:               utils.SanitizeAPIKey(config.GetString("api_key")),
		Site:                 config.GetString("site"),
		RegistryOverride:     config.GetString("updater.registry"),
		RegistryAuthOverride: authOverridesFromString(config.GetString("updater.registry_auth")),
	}
}

// ToEnv returns a slice of environment variables from the Env struct.
func (e *Env) ToEnv() []string {
	env := []string{
		envAPIKey + "=" + e.APIKey,
		envSite + "=" + e.Site,
		envRegistry + "=" + e.RegistryOverride,
		envRegistryAuth + "=" + authOverridesToEnv(e.RegistryAuthOverride),
	}
	env = append(env, overridesByPackageToEnv(envRegistry, e.RegistryOverrideByPackage)...)
	env = append(env, overridesByPackageToEnv(envDefaultVersion, e.DefaultVersionOverrideByPackage)...)
	return env
}

func overridesByPackageFromEnv(envPrefix string) map[string]string {
	env := os.Environ()
	overridesByPackage := map[string]string{}
	for _, e := range env {
		if strings.HasPrefix(e, envPrefix) {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) != 2 {
				continue
			}
			pkg := strings.TrimPrefix(parts[0], envPrefix+"_")
			pkg = strings.ToLower(pkg)
			pkg = strings.ReplaceAll(pkg, "_", "-")
			overridesByPackage[pkg] = parts[1]
		}
	}
	return overridesByPackage
}

func overridesByPackageToEnv(envPrefix string, overridesByPackage map[string]string) []string {
	env := []string{}
	for pkg, override := range overridesByPackage {
		pkg = strings.ReplaceAll(pkg, "-", "_")
		pkg = strings.ToUpper(pkg)
		env = append(env, envPrefix+"_"+pkg+"="+override)
	}
	return env
}

func authOverridesFromString(rawAuthOverrides string) []string {
	if rawAuthOverrides == "" {
		return defaultEnv.RegistryAuthOverride
	}
	return strings.Split(rawAuthOverrides, ",")
}

func authOverridesToEnv(authOverrides []string) string {
	return strings.Join(authOverrides, ",")
}

func getEnvOrDefault(env string, defaultValue string) string {
	value := os.Getenv(env)
	if value == "" {
		return defaultValue
	}
	return value
}
