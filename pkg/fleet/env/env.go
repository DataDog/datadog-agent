// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package env provides the environment variables for the installer.
package env

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

const (
	envAPIKey                = "DD_API_KEY"
	envSite                  = "DD_SITE"
	envRemoteUpdates         = "DD_REMOTE_UPDATES"
	envRemotePolicies        = "DD_REMOTE_POLICIES"
	envRegistryURL           = "DD_INSTALLER_REGISTRY_URL"
	envRegistryAuth          = "DD_INSTALLER_REGISTRY_AUTH"
	envRegistryUsername      = "DD_INSTALLER_REGISTRY_USERNAME"
	envRegistryPassword      = "DD_INSTALLER_REGISTRY_PASSWORD"
	envDefaultPackageVersion = "DD_INSTALLER_DEFAULT_PKG_VERSION"
	envDefaultPackageInstall = "DD_INSTALLER_DEFAULT_PKG_INSTALL"
	envApmLibraries          = "DD_APM_INSTRUMENTATION_LIBRARIES"
	envAgentMajorVersion     = "DD_AGENT_MAJOR_VERSION"
	envAgentMinorVersion     = "DD_AGENT_MINOR_VERSION"
	envApmLanguages          = "DD_APM_INSTRUMENTATION_LANGUAGES"
	envCDNLocalDirPath       = "DD_INSTALLER_DEBUG_CDN_LOCAL_DIR_PATH"
	envCDNEnabled            = "DD_INSTALLER_CDN_ENABLED"
)

var defaultEnv = Env{
	APIKey:        "",
	Site:          "datadoghq.com",
	RemoteUpdates: false,

	RegistryOverride:            "",
	RegistryAuthOverride:        "",
	RegistryUsername:            "",
	RegistryPassword:            "",
	RegistryOverrideByImage:     map[string]string{},
	RegistryAuthOverrideByImage: map[string]string{},
	RegistryUsernameByImage:     map[string]string{},
	RegistryPasswordByImage:     map[string]string{},

	DefaultPackagesInstallOverride: map[string]bool{},
	DefaultPackagesVersionOverride: map[string]string{},

	InstallScript: InstallScriptEnv{
		APMInstrumentationEnabled: "",
	},
}

// ApmLibLanguage is a language defined in DD_APM_INSTRUMENTATION_LIBRARIES env var
type ApmLibLanguage string

// ApmLibVersion is the version of the library defined in DD_APM_INSTRUMENTATION_LIBRARIES env var
type ApmLibVersion string

// Env contains the configuration for the installer.
type Env struct {
	APIKey         string
	Site           string
	RemoteUpdates  bool
	RemotePolicies bool

	RegistryOverride            string
	RegistryAuthOverride        string
	RegistryUsername            string
	RegistryPassword            string
	RegistryOverrideByImage     map[string]string
	RegistryAuthOverrideByImage map[string]string
	RegistryUsernameByImage     map[string]string
	RegistryPasswordByImage     map[string]string

	DefaultPackagesInstallOverride map[string]bool
	DefaultPackagesVersionOverride map[string]string

	ApmLibraries map[ApmLibLanguage]ApmLibVersion

	AgentMajorVersion string
	AgentMinorVersion string

	InstallScript InstallScriptEnv

	CDNEnabled      bool
	CDNLocalDirPath string
}

// FromEnv returns an Env struct with values from the environment.
func FromEnv() *Env {
	return &Env{
		APIKey:         getEnvOrDefault(envAPIKey, defaultEnv.APIKey),
		Site:           getEnvOrDefault(envSite, defaultEnv.Site),
		RemoteUpdates:  strings.ToLower(os.Getenv(envRemoteUpdates)) == "true",
		RemotePolicies: strings.ToLower(os.Getenv(envRemotePolicies)) == "true",

		RegistryOverride:            getEnvOrDefault(envRegistryURL, defaultEnv.RegistryOverride),
		RegistryAuthOverride:        getEnvOrDefault(envRegistryAuth, defaultEnv.RegistryAuthOverride),
		RegistryUsername:            getEnvOrDefault(envRegistryUsername, defaultEnv.RegistryUsername),
		RegistryPassword:            getEnvOrDefault(envRegistryPassword, defaultEnv.RegistryPassword),
		RegistryOverrideByImage:     overridesByNameFromEnv(envRegistryURL, func(s string) string { return s }),
		RegistryAuthOverrideByImage: overridesByNameFromEnv(envRegistryAuth, func(s string) string { return s }),
		RegistryUsernameByImage:     overridesByNameFromEnv(envRegistryUsername, func(s string) string { return s }),
		RegistryPasswordByImage:     overridesByNameFromEnv(envRegistryPassword, func(s string) string { return s }),

		DefaultPackagesInstallOverride: overridesByNameFromEnv(envDefaultPackageInstall, func(s string) bool { return strings.ToLower(s) == "true" }),
		DefaultPackagesVersionOverride: overridesByNameFromEnv(envDefaultPackageVersion, func(s string) string { return s }),

		ApmLibraries: parseApmLibrariesEnv(),

		AgentMajorVersion: os.Getenv(envAgentMajorVersion),
		AgentMinorVersion: os.Getenv(envAgentMinorVersion),

		InstallScript: installScriptEnvFromEnv(),

		CDNEnabled:      strings.ToLower(os.Getenv(envCDNEnabled)) == "true",
		CDNLocalDirPath: getEnvOrDefault(envCDNLocalDirPath, ""),
	}
}

// FromConfig returns an Env struct with values from the configuration.
func FromConfig(config model.Reader) *Env {
	return &Env{
		APIKey:               utils.SanitizeAPIKey(config.GetString("api_key")),
		Site:                 config.GetString("site"),
		RemoteUpdates:        config.GetBool("remote_updates"),
		RemotePolicies:       config.GetBool("remote_policies"),
		RegistryOverride:     config.GetString("installer.registry.url"),
		RegistryAuthOverride: config.GetString("installer.registry.auth"),
		RegistryUsername:     config.GetString("installer.registry.username"),
		RegistryPassword:     config.GetString("installer.registry.password"),
	}
}

// ToEnv returns a slice of environment variables from the Env struct.
func (e *Env) ToEnv() []string {
	var env []string
	if e.APIKey != "" {
		env = append(env, envAPIKey+"="+e.APIKey)
	}
	if e.Site != "" {
		env = append(env, envSite+"="+e.Site)
	}
	if e.RemoteUpdates {
		env = append(env, envRemoteUpdates+"=true")
	}
	if e.RemotePolicies {
		env = append(env, envRemotePolicies+"=true")
	}
	if e.RegistryOverride != "" {
		env = append(env, envRegistryURL+"="+e.RegistryOverride)
	}
	if e.RegistryAuthOverride != "" {
		env = append(env, envRegistryAuth+"="+e.RegistryAuthOverride)
	}
	if e.RegistryUsername != "" {
		env = append(env, envRegistryUsername+"="+e.RegistryUsername)
	}
	if e.RegistryPassword != "" {
		env = append(env, envRegistryPassword+"="+e.RegistryPassword)
	}
	if len(e.ApmLibraries) > 0 {
		libraries := []string{}
		for l, v := range e.ApmLibraries {
			l := string(l)
			if v != "" {
				l = l + ":" + string(v)
			}
			libraries = append(libraries, l)
		}
		slices.Sort(libraries)
		env = append(env, envApmLibraries+"="+strings.Join(libraries, ","))
	}
	env = append(env, overridesByNameToEnv(envRegistryURL, e.RegistryOverrideByImage)...)
	env = append(env, overridesByNameToEnv(envRegistryAuth, e.RegistryAuthOverrideByImage)...)
	env = append(env, overridesByNameToEnv(envRegistryUsername, e.RegistryUsernameByImage)...)
	env = append(env, overridesByNameToEnv(envRegistryPassword, e.RegistryPasswordByImage)...)
	env = append(env, overridesByNameToEnv(envDefaultPackageInstall, e.DefaultPackagesInstallOverride)...)
	env = append(env, overridesByNameToEnv(envDefaultPackageVersion, e.DefaultPackagesVersionOverride)...)
	return env
}

func parseApmLibrariesEnv() map[ApmLibLanguage]ApmLibVersion {
	apmLibraries, ok := os.LookupEnv(envApmLibraries)
	if !ok {
		return parseAPMLanguagesEnv()
	}
	apmLibrariesVersion := map[ApmLibLanguage]ApmLibVersion{}
	if apmLibraries == "" {
		return apmLibrariesVersion
	}
	for _, library := range strings.FieldsFunc(apmLibraries, func(r rune) bool {
		return r == ',' || r == ' '
	}) {
		libraryName, libraryVersion, _ := strings.Cut(library, ":")
		apmLibrariesVersion[ApmLibLanguage(libraryName)] = ApmLibVersion(libraryVersion)
	}
	return apmLibrariesVersion
}

func parseAPMLanguagesEnv() map[ApmLibLanguage]ApmLibVersion {
	apmLanguages := os.Getenv(envApmLanguages)
	res := map[ApmLibLanguage]ApmLibVersion{}
	for _, language := range strings.Split(apmLanguages, " ") {
		if len(language) > 0 {
			res[ApmLibLanguage(language)] = ""
		}
	}
	return res
}

func overridesByNameFromEnv[T any](envPrefix string, convert func(string) T) map[string]T {
	env := os.Environ()
	overridesByPackage := map[string]T{}
	for _, e := range env {
		keyVal := strings.SplitN(e, "=", 2)
		if len(keyVal) != 2 {
			continue
		}
		if strings.HasPrefix(keyVal[0], envPrefix+"_") {
			pkg := strings.TrimPrefix(keyVal[0], envPrefix+"_")
			pkg = strings.ToLower(pkg)
			pkg = strings.ReplaceAll(pkg, "_", "-")
			overridesByPackage[pkg] = convert(keyVal[1])
		}
	}
	return overridesByPackage
}

func overridesByNameToEnv[T any](envPrefix string, overridesByPackage map[string]T) []string {
	env := []string{}
	for pkg, override := range overridesByPackage {
		pkg = strings.ReplaceAll(pkg, "-", "_")
		pkg = strings.ToUpper(pkg)
		env = append(env, envPrefix+"_"+pkg+"="+fmt.Sprint(override))
	}
	return env
}

func getEnvOrDefault(env string, defaultValue string) string {
	value := os.Getenv(env)
	if value == "" {
		return defaultValue
	}
	return value
}
