// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package env

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The installer env package is env-var only. Any yaml-sourced configuration
// is the caller's responsibility to translate into DD_* env vars before
// invoking the installer (the daemon does it via its fx config.Component,
// the CLI via its fx bootstrap).

func TestGet(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected *Env
	}{
		{
			name:    "empty environment variables",
			envVars: map[string]string{},
			expected: func() *Env {
				e := newDefaultEnv()
				return &e
			}(),
		},
		{
			name: "environment variables only",
			envVars: map[string]string{
				"DD_API_KEY":                                       "123456",
				"DD_SITE":                                          "datadoghq.eu",
				"DD_REMOTE_UPDATES":                                "true",
				"DD_INSTALLER_MIRROR":                              "https://mirror.example.com",
				"DD_INSTALLER_REGISTRY_URL":                        "registry.example.com",
				"DD_INSTALLER_REGISTRY_AUTH":                       "auth",
				"DD_INSTALLER_REGISTRY_USERNAME":                   "username",
				"DD_INSTALLER_REGISTRY_PASSWORD":                   "password",
				"DD_INSTALLER_REGISTRY_URL_IMAGE":                  "another.registry.example.com",
				"DD_INSTALLER_REGISTRY_URL_ANOTHER_IMAGE":          "yet.another.registry.example.com",
				"DD_INSTALLER_REGISTRY_AUTH_IMAGE":                 "another.auth",
				"DD_INSTALLER_REGISTRY_AUTH_ANOTHER_IMAGE":         "yet.another.auth",
				"DD_INSTALLER_REGISTRY_USERNAME_IMAGE":             "another.username",
				"DD_INSTALLER_REGISTRY_USERNAME_ANOTHER_IMAGE":     "yet.another.username",
				"DD_INSTALLER_REGISTRY_PASSWORD_IMAGE":             "another.password",
				"DD_INSTALLER_REGISTRY_PASSWORD_ANOTHER_IMAGE":     "yet.another.password",
				"DD_INSTALLER_DEFAULT_PKG_INSTALL_PACKAGE":         "true",
				"DD_INSTALLER_DEFAULT_PKG_INSTALL_ANOTHER_PACKAGE": "false",
				"DD_INSTALLER_DEFAULT_PKG_VERSION_PACKAGE":         "1.2.3",
				"DD_INSTALLER_DEFAULT_PKG_VERSION_ANOTHER_PACKAGE": "4.5.6",
				"DD_APM_INSTRUMENTATION_LIBRARIES":                 "java,dotnet:latest,ruby:1.2",
				"DD_APM_INSTRUMENTATION_ENABLED":                   "all",
				"DD_AGENT_USER_NAME":                               "customuser",
				"DD_TAGS":                                          "k1:v1,k2:v2",
				"DD_EXTRA_TAGS":                                    "k3:v3,k4:v4",
				"DD_HOSTNAME":                                      "hostname",
				"DD_PROXY_HTTP":                                    "http://proxy.example.com:8080",
				"DD_PROXY_HTTPS":                                   "http://proxy.example.com:8080",
				"DD_PROXY_NO_PROXY":                                "localhost",
				"DD_INFRASTRUCTURE_MODE":                           "basic",
				"DD_APP_KEY":                                       "app_key_123",
				"DD_PRIVATE_ACTION_RUNNER_ENABLED":                 "true",
				"DD_PRIVATE_ACTION_RUNNER_ACTIONS_ALLOWLIST":       "com.datadoghq.script.runPredefinedScript,com.datadoghq.script.testConnection",
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
				},
				Tags:                       []string{"k1:v1", "k2:v2", "k3:v3", "k4:v4"},
				Hostname:                   "hostname",
				HTTPProxy:                  "http://proxy.example.com:8080",
				HTTPSProxy:                 "http://proxy.example.com:8080",
				NoProxy:                    "localhost",
				InfrastructureMode:         "basic",
				AppKey:                     "app_key_123",
				PAREnabled:                 true,
				PARActionsAllowlist:        "com.datadoghq.script.runPredefinedScript,com.datadoghq.script.testConnection",
				LogLevel:                   "warn",
				ExtensionRegistryOverrides: map[string]map[string]ExtensionRegistryOverride{},
			},
		},
		{
			name: "APM libraries parsing",
			envVars: map[string]string{
				"DD_APM_INSTRUMENTATION_LIBRARIES": "java  dotnet:latest, ruby:1.2   ,python:1.2.3",
			},
			expected: &Env{
				Site:                           "datadoghq.com",
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
				},
				Tags:                       []string{},
				LogLevel:                   "warn",
				ExtensionRegistryOverrides: map[string]map[string]ExtensionRegistryOverride{},
			},
		},
		{
			name: "deprecated apm lang",
			envVars: map[string]string{
				"DD_API_KEY":                       "123456",
				"DD_APM_INSTRUMENTATION_LANGUAGES": "java dotnet ruby",
				"DD_APM_INSTRUMENTATION_ENABLED":   "all",
			},
			expected: &Env{
				APIKey: "123456",
				Site:   "datadoghq.com",
				ApmLibraries: map[ApmLibLanguage]ApmLibVersion{
					"java":   "",
					"dotnet": "",
					"ruby":   "",
				},
				RegistryOverrideByImage:        map[string]string{},
				RegistryAuthOverrideByImage:    map[string]string{},
				RegistryUsernameByImage:        map[string]string{},
				RegistryPasswordByImage:        map[string]string{},
				DefaultPackagesInstallOverride: map[string]bool{},
				DefaultPackagesVersionOverride: map[string]string{},
				InstallScript: InstallScriptEnv{
					APMInstrumentationEnabled: APMInstrumentationEnabledAll,
				},
				Tags:                       []string{},
				LogLevel:                   "warn",
				ExtensionRegistryOverrides: map[string]map[string]ExtensionRegistryOverride{},
			},
		},
		{
			name: "primary set",
			envVars: map[string]string{
				"DD_AGENT_USER_NAME": "customuser",
			},
			expected: &Env{
				Site: "datadoghq.com",
				MsiParams: MsiParamsEnv{
					AgentUserName: "customuser",
				},
				RegistryOverrideByImage:        map[string]string{},
				RegistryAuthOverrideByImage:    map[string]string{},
				RegistryUsernameByImage:        map[string]string{},
				RegistryPasswordByImage:        map[string]string{},
				DefaultPackagesInstallOverride: map[string]bool{},
				DefaultPackagesVersionOverride: map[string]string{},
				ApmLibraries:                   map[ApmLibLanguage]ApmLibVersion{},
				InstallScript: InstallScriptEnv{
					APMInstrumentationEnabled: APMInstrumentationNotSet,
				},
				Tags:                       []string{},
				LogLevel:                   "warn",
				ExtensionRegistryOverrides: map[string]map[string]ExtensionRegistryOverride{},
			},
		},
		{
			name: "compat set",
			envVars: map[string]string{
				"DDAGENTUSER_NAME": "customuser",
			},
			expected: &Env{
				Site: "datadoghq.com",
				MsiParams: MsiParamsEnv{
					AgentUserName: "customuser",
				},
				RegistryOverrideByImage:        map[string]string{},
				RegistryAuthOverrideByImage:    map[string]string{},
				RegistryUsernameByImage:        map[string]string{},
				RegistryPasswordByImage:        map[string]string{},
				DefaultPackagesInstallOverride: map[string]bool{},
				DefaultPackagesVersionOverride: map[string]string{},
				ApmLibraries:                   map[ApmLibLanguage]ApmLibVersion{},
				InstallScript: InstallScriptEnv{
					APMInstrumentationEnabled: APMInstrumentationNotSet,
				},
				Tags:                       []string{},
				LogLevel:                   "warn",
				ExtensionRegistryOverrides: map[string]map[string]ExtensionRegistryOverride{},
			},
		},
		{
			name: "primary precedence",
			envVars: map[string]string{
				"DD_AGENT_USER_NAME": "customuser",
				"DDAGENTUSER_NAME":   "otheruser",
			},
			expected: &Env{
				Site: "datadoghq.com",
				MsiParams: MsiParamsEnv{
					AgentUserName: "customuser",
				},
				RegistryOverrideByImage:        map[string]string{},
				RegistryAuthOverrideByImage:    map[string]string{},
				RegistryUsernameByImage:        map[string]string{},
				RegistryPasswordByImage:        map[string]string{},
				DefaultPackagesInstallOverride: map[string]bool{},
				DefaultPackagesVersionOverride: map[string]string{},
				ApmLibraries:                   map[ApmLibLanguage]ApmLibVersion{},
				InstallScript: InstallScriptEnv{
					APMInstrumentationEnabled: APMInstrumentationNotSet,
				},
				Tags:                       []string{},
				LogLevel:                   "warn",
				ExtensionRegistryOverrides: map[string]map[string]ExtensionRegistryOverride{},
			},
		},
	}

	savedEnv := make(map[string]string)
	for _, kv := range os.Environ() {
		key, val, _ := strings.Cut(kv, "=")
		savedEnv[key] = val
		os.Unsetenv(key)
	}

	defer func() {
		for k, v := range savedEnv {
			os.Setenv(k, v)
		}
	}()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.envVars {
				t.Setenv(k, v)
			}

			env := Get()

			assert.Equal(t, tc.expected, env)
		})
	}
}

// TestGet_extensionRegistryOverridesFromEnv exercises the
// DD_INSTALLER_REGISTRY_EXT_*_<PKG>__<EXT> prefix pattern that the daemon
// / CLI uses to forward per-extension registry overrides to the installer.
func TestGet_extensionRegistryOverridesFromEnv(t *testing.T) {
	savedEnv := make(map[string]string)
	for _, kv := range os.Environ() {
		key, val, _ := strings.Cut(kv, "=")
		savedEnv[key] = val
		os.Unsetenv(key)
	}
	defer func() {
		for k, v := range savedEnv {
			os.Setenv(k, v)
		}
	}()

	t.Setenv("DD_INSTALLER_REGISTRY_EXT_URL_DATADOG_AGENT__DDOT", "custom.example.com")
	t.Setenv("DD_INSTALLER_REGISTRY_EXT_AUTH_DATADOG_AGENT__DDOT", "basic")
	t.Setenv("DD_INSTALLER_REGISTRY_EXT_USERNAME_DATADOG_AGENT__DDOT", "u")
	t.Setenv("DD_INSTALLER_REGISTRY_EXT_PASSWORD_DATADOG_AGENT__DDOT", "p")
	t.Setenv("DD_INSTALLER_REGISTRY_EXT_URL_DATADOG_AGENT__OTHER_EXT", "other.example.com")

	env := Get()

	expected := map[string]map[string]ExtensionRegistryOverride{
		"datadog-agent": {
			"ddot": {
				URL:      "custom.example.com",
				Auth:     "basic",
				Username: "u",
				Password: "p",
			},
			"other-ext": {
				URL: "other.example.com",
			},
		},
	}
	assert.Equal(t, expected, env.ExtensionRegistryOverrides)
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
				Tags:                []string{"k1:v1", "k2:v2"},
				Hostname:            "hostname",
				HTTPProxy:           "http://proxy.example.com:8080",
				HTTPSProxy:          "http://proxy.example.com:8080",
				NoProxy:             "localhost",
				InfrastructureMode:  "end_user_device",
				PAREnabled:          true,
				AppKey:              "app_key_123",
				PARActionsAllowlist: "action1,action2",
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
				"DD_APP_KEY=app_key_123",
				"DD_PRIVATE_ACTION_RUNNER_ENABLED=true",
				"DD_PRIVATE_ACTION_RUNNER_ACTIONS_ALLOWLIST=action1,action2",
			},
		},
		{
			name: "PAR enabled without app key",
			env: &Env{
				APIKey:              "123456",
				PAREnabled:          true,
				PARActionsAllowlist: "action1,action2",
			},
			expected: []string{
				"DD_API_KEY=123456",
				"DD_PRIVATE_ACTION_RUNNER_ENABLED=true",
				"DD_PRIVATE_ACTION_RUNNER_ACTIONS_ALLOWLIST=action1,action2",
			},
		},
		{
			name: "PAR disabled does not emit PAR env vars",
			env: &Env{
				APIKey:              "123456",
				PAREnabled:          false,
				AppKey:              "app_key_123",
				PARActionsAllowlist: "action1",
			},
			expected: []string{
				"DD_API_KEY=123456",
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

// TestToEnv_emitsExtensionRegistryOverrides covers the round-trip
// encoding of ExtensionRegistryOverrides onto the
// DD_INSTALLER_REGISTRY_EXT_*_<PKG>__<EXT> prefix scheme.
func TestToEnv_emitsExtensionRegistryOverrides(t *testing.T) {
	e := &Env{
		APIKey: "123456",
		ExtensionRegistryOverrides: map[string]map[string]ExtensionRegistryOverride{
			"datadog-agent": {
				"ddot": {
					URL:      "custom.example.com",
					Auth:     "basic",
					Username: "u",
					Password: "p",
				},
				"other-ext": {URL: "other.example.com"},
			},
		},
	}

	result := e.ToEnv()

	assert.ElementsMatch(t, []string{
		"DD_API_KEY=123456",
		"DD_INSTALLER_REGISTRY_EXT_URL_DATADOG_AGENT__DDOT=custom.example.com",
		"DD_INSTALLER_REGISTRY_EXT_AUTH_DATADOG_AGENT__DDOT=basic",
		"DD_INSTALLER_REGISTRY_EXT_USERNAME_DATADOG_AGENT__DDOT=u",
		"DD_INSTALLER_REGISTRY_EXT_PASSWORD_DATADOG_AGENT__DDOT=p",
		"DD_INSTALLER_REGISTRY_EXT_URL_DATADOG_AGENT__OTHER_EXT=other.example.com",
	}, result)
}
