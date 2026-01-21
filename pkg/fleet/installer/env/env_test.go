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
				Mirror:                         "",
				RegistryOverride:               "",
				RegistryAuthOverride:           "",
				RegistryUsername:               "",
				RegistryPassword:               "",
				RegistryOverrideByImage:        map[string]string{},
				RegistryAuthOverrideByImage:    map[string]string{},
				RegistryUsernameByImage:        map[string]string{},
				RegistryPasswordByImage:        map[string]string{},
				DefaultPackagesInstallOverride: map[string]bool{},
				DefaultPackagesVersionOverride: map[string]string{},
				ApmLibraries:                   map[ApmLibLanguage]ApmLibVersion{},
				InstallScript: InstallScriptEnv{
					APMInstrumentationEnabled: APMInstrumentationNotSet,
					APMInjectorMode:           APMInjectorModeDirect,
				},
				Tags:       []string{},
				Hostname:   "",
				HTTPProxy:  "",
				HTTPSProxy: "",
				NoProxy:    os.Getenv("NO_PROXY"), // Default value from the environment, as some test envs set it
			},
		},
		{
			name: "All environment variables set",
			envVars: map[string]string{
				envAPIKey:                                     "123456",
				envSite:                                       "datadoghq.eu",
				envRemoteUpdates:                              "true",
				envMirror:                                     "https://mirror.example.com",
				envRegistryURL:                                "registry.example.com",
				envRegistryAuth:                               "auth",
				envRegistryUsername:                           "username",
				envRegistryPassword:                           "password",
				envRegistryURL + "_IMAGE":                     "another.registry.example.com",
				envRegistryURL + "_ANOTHER_IMAGE":             "yet.another.registry.example.com",
				envRegistryAuth + "_IMAGE":                    "another.auth",
				envRegistryAuth + "_ANOTHER_IMAGE":            "yet.another.auth",
				envRegistryUsername + "_IMAGE":                "another.username",
				envRegistryUsername + "_ANOTHER_IMAGE":        "yet.another.username",
				envRegistryPassword + "_IMAGE":                "another.password",
				envRegistryPassword + "_ANOTHER_IMAGE":        "yet.another.password",
				envDefaultPackageInstall + "_PACKAGE":         "true",
				envDefaultPackageInstall + "_ANOTHER_PACKAGE": "false",
				envDefaultPackageVersion + "_PACKAGE":         "1.2.3",
				envDefaultPackageVersion + "_ANOTHER_PACKAGE": "4.5.6",
				envApmLibraries:                               "java,dotnet:latest,ruby:1.2",
				envApmInstrumentationEnabled:                  "all",
				envAgentUserName:                              "customuser",
				envTags:                                       "k1:v1,k2:v2",
				envExtraTags:                                  "k3:v3,k4:v4",
				envHostname:                                   "hostname",
				envDDHTTPProxy:                                "http://proxy.example.com:8080",
				envDDHTTPSProxy:                               "http://proxy.example.com:8080",
				envDDNoProxy:                                  "localhost",
				envInfrastructureMode:                         "basic",
			},
			expected: &Env{
				APIKey:               "123456",
				Site:                 "datadoghq.eu",
				Mirror:               "https://mirror.example.com",
				RemoteUpdates:        true,
				RegistryOverride:     "registry.example.com",
				RegistryAuthOverride: "auth",
				RegistryUsername:     "username",
				RegistryPassword:     "password",
				RegistryOverrideByImage: map[string]string{
					"image":         "another.registry.example.com",
					"another-image": "yet.another.registry.example.com",
				},
				RegistryAuthOverrideByImage: map[string]string{
					"image":         "another.auth",
					"another-image": "yet.another.auth",
				},
				RegistryUsernameByImage: map[string]string{
					"image":         "another.username",
					"another-image": "yet.another.username",
				},
				RegistryPasswordByImage: map[string]string{
					"image":         "another.password",
					"another-image": "yet.another.password",
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
				MsiParams: MsiParamsEnv{
					AgentUserName: "customuser",
				},
				InstallScript: InstallScriptEnv{
					APMInstrumentationEnabled: APMInstrumentationEnabledAll,
					APMInjectorMode:           APMInjectorModeDirect,
				},
				Tags:               []string{"k1:v1", "k2:v2", "k3:v3", "k4:v4"},
				Hostname:           "hostname",
				HTTPProxy:          "http://proxy.example.com:8080",
				HTTPSProxy:         "http://proxy.example.com:8080",
				NoProxy:            "localhost",
				InfrastructureMode: "basic",
			},
		},
		{
			name: "APM libraries parsing",
			envVars: map[string]string{
				envApmLibraries: "java  dotnet:latest, ruby:1.2   ,python:1.2.3",
			},
			expected: &Env{
				APIKey:                         "",
				Site:                           "datadoghq.com",
				RegistryOverride:               "",
				RegistryAuthOverride:           "",
				RegistryOverrideByImage:        map[string]string{},
				RegistryAuthOverrideByImage:    map[string]string{},
				RegistryUsernameByImage:        map[string]string{},
				RegistryPasswordByImage:        map[string]string{},
				DefaultPackagesInstallOverride: map[string]bool{},
				DefaultPackagesVersionOverride: map[string]string{},
				ApmLibraries: map[ApmLibLanguage]ApmLibVersion{
					"java":   "",
					"dotnet": "latest",
					"ruby":   "1.2",
					"python": "1.2.3",
				},
				InstallScript: InstallScriptEnv{
					APMInstrumentationEnabled: APMInstrumentationNotSet,
					APMInjectorMode:           APMInjectorModeDirect,
				},
				Tags:    []string{},
				NoProxy: os.Getenv("NO_PROXY"), // Default value from the environment, as some test envs set it
			},
		},
		{
			name: "deprecated apm lang",
			envVars: map[string]string{
				envAPIKey:                    "123456",
				envApmLanguages:              "java dotnet ruby",
				envApmInstrumentationEnabled: "all",
			},
			expected: &Env{
				APIKey: "123456",
				Site:   "datadoghq.com",
				ApmLibraries: map[ApmLibLanguage]ApmLibVersion{
					"java":   "",
					"dotnet": "",
					"ruby":   "",
				},
				InstallScript: InstallScriptEnv{
					APMInstrumentationEnabled: APMInstrumentationEnabledAll,
					APMInjectorMode:           APMInjectorModeDirect,
				},
				RegistryOverrideByImage:        map[string]string{},
				RegistryAuthOverrideByImage:    map[string]string{},
				RegistryUsernameByImage:        map[string]string{},
				RegistryPasswordByImage:        map[string]string{},
				DefaultPackagesInstallOverride: map[string]bool{},
				DefaultPackagesVersionOverride: map[string]string{},
				Tags:                           []string{},
				Hostname:                       "",
				NoProxy:                        os.Getenv("NO_PROXY"), // Default value from the environment, as some test envs set it
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
				Mirror:               "https://mirror.example.com",
				RegistryOverride:     "registry.example.com",
				RegistryAuthOverride: "auth",
				RegistryUsername:     "username",
				RegistryPassword:     "password",
				RegistryOverrideByImage: map[string]string{
					"image":         "another.registry.example.com",
					"another-image": "yet.another.registry.example.com",
				},
				RegistryAuthOverrideByImage: map[string]string{
					"image":         "another.auth",
					"another-image": "yet.another.auth",
				},
				RegistryUsernameByImage: map[string]string{
					"image":         "another.username",
					"another-image": "yet.another.username",
				},
				RegistryPasswordByImage: map[string]string{
					"image":         "another.password",
					"another-image": "yet.another.password",
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
				Tags:               []string{"k1:v1", "k2:v2"},
				Hostname:           "hostname",
				HTTPProxy:          "http://proxy.example.com:8080",
				HTTPSProxy:         "http://proxy.example.com:8080",
				NoProxy:            "localhost",
				InfrastructureMode: "end_user_device",
			},
			expected: []string{
				"DD_API_KEY=123456",
				"DD_SITE=datadoghq.eu",
				"DD_REMOTE_UPDATES=true",
				"DD_INSTALLER_MIRROR=https://mirror.example.com",
				"DD_INSTALLER_REGISTRY_URL=registry.example.com",
				"DD_INSTALLER_REGISTRY_AUTH=auth",
				"DD_INSTALLER_REGISTRY_USERNAME=username",
				"DD_INSTALLER_REGISTRY_PASSWORD=password",
				"DD_APM_INSTRUMENTATION_LIBRARIES=dotnet:latest,java,ruby:1.2",
				"DD_INSTALLER_REGISTRY_URL_IMAGE=another.registry.example.com",
				"DD_INSTALLER_REGISTRY_URL_ANOTHER_IMAGE=yet.another.registry.example.com",
				"DD_INSTALLER_REGISTRY_AUTH_IMAGE=another.auth",
				"DD_INSTALLER_REGISTRY_AUTH_ANOTHER_IMAGE=yet.another.auth",
				"DD_INSTALLER_REGISTRY_USERNAME_IMAGE=another.username",
				"DD_INSTALLER_REGISTRY_USERNAME_ANOTHER_IMAGE=yet.another.username",
				"DD_INSTALLER_REGISTRY_PASSWORD_IMAGE=another.password",
				"DD_INSTALLER_REGISTRY_PASSWORD_ANOTHER_IMAGE=yet.another.password",
				"DD_INSTALLER_DEFAULT_PKG_INSTALL_PACKAGE=true",
				"DD_INSTALLER_DEFAULT_PKG_INSTALL_ANOTHER_PACKAGE=false",
				"DD_INSTALLER_DEFAULT_PKG_VERSION_PACKAGE=1.2.3",
				"DD_INSTALLER_DEFAULT_PKG_VERSION_ANOTHER_PACKAGE=4.5.6",
				"DD_TAGS=k1:v1,k2:v2",
				"DD_HOSTNAME=hostname",
				"HTTP_PROXY=http://proxy.example.com:8080",
				"HTTPS_PROXY=http://proxy.example.com:8080",
				"NO_PROXY=localhost",
				"DD_INFRASTRUCTURE_MODE=end_user_device",
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

func TestAgentUserVars(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected *Env
	}{
		{
			name:    "not set",
			envVars: map[string]string{},
			expected: &Env{
				MsiParams: MsiParamsEnv{
					AgentUserName: "",
				},
			},
		},
		{
			name: "primary set",
			envVars: map[string]string{
				envAgentUserName: "customuser",
			},
			expected: &Env{
				MsiParams: MsiParamsEnv{
					AgentUserName: "customuser",
				},
			},
		},
		{
			name: "compat set",
			envVars: map[string]string{
				envAgentUserNameCompat: "customuser",
			},
			expected: &Env{
				MsiParams: MsiParamsEnv{
					AgentUserName: "customuser",
				},
			},
		},
		{
			name: "primary precedence",
			envVars: map[string]string{
				envAgentUserName:       "customuser",
				envAgentUserNameCompat: "otheruser",
			},
			expected: &Env{
				MsiParams: MsiParamsEnv{
					AgentUserName: "customuser",
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
			assert.Equal(t, tt.expected.MsiParams.AgentUserName, result.MsiParams.AgentUserName)
		})
	}
}

func TestAPMInjectorMode_FromEnv(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected string
	}{
		{
			name:     "default mode (not set)",
			envValue: "",
			expected: APMInjectorModeDirect,
		},
		{
			name:     "direct mode",
			envValue: "direct",
			expected: APMInjectorModeDirect,
		},
		{
			name:     "service mode",
			envValue: "service",
			expected: APMInjectorModeService,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv("DD_APM_INJECTOR_MODE", tt.envValue)
				defer os.Unsetenv("DD_APM_INJECTOR_MODE")
			}

			env := FromEnv()
			assert.Equal(t, tt.expected, env.InstallScript.APMInjectorMode)
		})
	}
}

func TestValidateAPMInjectorMode(t *testing.T) {
	tests := []struct {
		name        string
		mode        string
		expectError bool
	}{
		{
			name:        "valid direct mode",
			mode:        APMInjectorModeDirect,
			expectError: false,
		},
		{
			name:        "valid service mode",
			mode:        APMInjectorModeService,
			expectError: false,
		},
		{
			name:        "invalid mode",
			mode:        "invalid",
			expectError: true,
		},
		{
			name:        "empty mode",
			mode:        "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAPMInjectorMode(tt.mode)
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid value for DD_APM_INJECTOR_MODE")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestInstallScriptEnv_ToEnv_WithAPMInjectorMode(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		expected string
	}{
		{
			name:     "direct mode",
			mode:     APMInjectorModeDirect,
			expected: "DD_APM_INJECTOR_MODE=direct",
		},
		{
			name:     "service mode",
			mode:     APMInjectorModeService,
			expected: "DD_APM_INJECTOR_MODE=service",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scriptEnv := InstallScriptEnv{
				APMInjectorMode: tt.mode,
			}

			envVars := scriptEnv.ToEnv(nil)

			found := false
			for _, env := range envVars {
				if env == tt.expected {
					found = true
					break
				}
			}
			assert.True(t, found, "Expected env var %s not found in %v", tt.expected, envVars)
		})
	}
}

func TestAPMInjectorModeConstants(t *testing.T) {
	// Verify constants have expected values
	assert.Equal(t, "direct", APMInjectorModeDirect)
	assert.Equal(t, "service", APMInjectorModeService)
}

func TestDefaultEnv_APMInjectorMode(t *testing.T) {
	// Verify default environment has direct mode
	assert.Equal(t, APMInjectorModeDirect, defaultEnv.InstallScript.APMInjectorMode)
}
