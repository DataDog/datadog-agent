// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package env

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected *Env
	}{
		{
			name:    "Empty environment variables",
			envVars: map[string]string{},
			expected: &Env{
				APIKey:                         "",
				Site:                           "datadoghq.com",
				RegistryOverride:               "",
				RegistryAuthOverride:           "",
				RegistryOverrideByImage:        map[string]string{},
				RegistryAuthOverrideByImage:    map[string]string{},
				DefaultPackagesInstallOverride: map[string]bool{},
				DefaultPackagesVersionOverride: map[string]string{},
				ApmLibraries:                   map[ApmLibLanguage]ApmLibVersion{},
				InstallScript: InstallScriptEnv{
					APMInstrumentationEnabled: APMInstrumentationNotSet,
				},
			},
		},
		{
			name: "All environment variables set",
			envVars: map[string]string{
				envAPIKey:                                     "123456",
				envSite:                                       "datadoghq.eu",
				envRemoteUpdates:                              "true",
				envRegistryURL:                                "registry.example.com",
				envRegistryAuth:                               "auth",
				envRegistryURL + "_IMAGE":                     "another.registry.example.com",
				envRegistryURL + "_ANOTHER_IMAGE":             "yet.another.registry.example.com",
				envRegistryAuth + "_IMAGE":                    "another.auth",
				envRegistryAuth + "_ANOTHER_IMAGE":            "yet.another.auth",
				envDefaultPackageInstall + "_PACKAGE":         "true",
				envDefaultPackageInstall + "_ANOTHER_PACKAGE": "false",
				envDefaultPackageVersion + "_PACKAGE":         "1.2.3",
				envDefaultPackageVersion + "_ANOTHER_PACKAGE": "4.5.6",
				envApmLibraries:                               "java,dotnet:latest,ruby:1.2",
				envApmInstrumentationEnabled:                  "all",
			},
			expected: &Env{
				APIKey:               "123456",
				Site:                 "datadoghq.eu",
				RemoteUpdates:        true,
				RegistryOverride:     "registry.example.com",
				RegistryAuthOverride: "auth",
				RegistryOverrideByImage: map[string]string{
					"image":         "another.registry.example.com",
					"another-image": "yet.another.registry.example.com",
				},
				RegistryAuthOverrideByImage: map[string]string{
					"image":         "another.auth",
					"another-image": "yet.another.auth",
				},
				DefaultPackagesInstallOverride: map[string]bool{
					"package":         true,
					"another-package": false,
				},
				DefaultPackagesVersionOverride: map[string]string{
					"package":         "1.2.3",
					"another-package": "4.5.6",
				},
				ApmLibraries: map[ApmLibLanguage]ApmLibVersion{
					"java":   "",
					"dotnet": "latest",
					"ruby":   "1.2",
				},
				InstallScript: InstallScriptEnv{
					APMInstrumentationEnabled: APMInstrumentationEnabledAll,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.envVars {
				os.Setenv(key, value)
				defer os.Unsetenv(key)
			}
			result := FromEnv()
			assert.Equal(t, tt.expected, result, "failed %v", tt.name)
		})
	}
}

func TestToEnv(t *testing.T) {
	tests := []struct {
		name     string
		env      *Env
		expected []string
	}{
		{
			name:     "Empty configuration",
			env:      &Env{},
			expected: nil,
		},
		{
			name: "All configuration set",
			env: &Env{
				APIKey:               "123456",
				Site:                 "datadoghq.eu",
				RemoteUpdates:        true,
				RegistryOverride:     "registry.example.com",
				RegistryAuthOverride: "auth",
				RegistryOverrideByImage: map[string]string{
					"image":         "another.registry.example.com",
					"another-image": "yet.another.registry.example.com",
				},
				RegistryAuthOverrideByImage: map[string]string{
					"image":         "another.auth",
					"another-image": "yet.another.auth",
				},
				DefaultPackagesInstallOverride: map[string]bool{
					"package":         true,
					"another-package": false,
				},
				DefaultPackagesVersionOverride: map[string]string{
					"package":         "1.2.3",
					"another-package": "4.5.6",
				},
				ApmLibraries: map[ApmLibLanguage]ApmLibVersion{
					"java":   "",
					"dotnet": "latest",
					"ruby":   "1.2",
				},
			},
			expected: []string{
				"DD_API_KEY=123456",
				"DD_SITE=datadoghq.eu",
				"DD_REMOTE_UPDATES=true",
				"DD_INSTALLER_REGISTRY_URL=registry.example.com",
				"DD_INSTALLER_REGISTRY_AUTH=auth",
				"DD_APM_INSTRUMENTATION_LIBRARIES=dotnet:latest,java,ruby:1.2",
				"DD_INSTALLER_REGISTRY_URL_IMAGE=another.registry.example.com",
				"DD_INSTALLER_REGISTRY_URL_ANOTHER_IMAGE=yet.another.registry.example.com",
				"DD_INSTALLER_REGISTRY_AUTH_IMAGE=another.auth",
				"DD_INSTALLER_REGISTRY_AUTH_ANOTHER_IMAGE=yet.another.auth",
				"DD_INSTALLER_DEFAULT_PKG_INSTALL_PACKAGE=true",
				"DD_INSTALLER_DEFAULT_PKG_INSTALL_ANOTHER_PACKAGE=false",
				"DD_INSTALLER_DEFAULT_PKG_VERSION_PACKAGE=1.2.3",
				"DD_INSTALLER_DEFAULT_PKG_VERSION_ANOTHER_PACKAGE=4.5.6",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.env.ToEnv()
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}
