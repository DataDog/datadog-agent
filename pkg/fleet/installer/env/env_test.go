// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package env

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeDatadogYAML(t *testing.T, dir string, content string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(dir, "datadog.yaml"), []byte(content), 0644)
	require.NoError(t, err)
}

func TestGet(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		envVars  map[string]string
		expected *Env
	}{
		{
			name:    "empty environment variables and config",
			envVars: map[string]string{},
			expected: func() *Env {
				e := newDefaultEnv()
				return &e
			}(),
		},
		{
			name: "environment variables only",
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
				envAppKey:                                     "app_key_123",
				envPAREnabled:                                 "true",
				envPARActionsAllowlist:                        "com.datadoghq.script.runPredefinedScript,com.datadoghq.script.testConnection",
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
				ExtensionRegistryOverrides: map[string]map[string]ExtensionRegistryOverride{},
			},
		},
		{
			name:    "config only",
			envVars: map[string]string{},
			yaml: `
api_key: yaml-api-key
site: yaml-site.example.com
installer:
  registry:
    url: yaml-registry.example.com
    auth: yaml-auth
    username: yaml-user
    password: yaml-pass
    extensions:
      datadog-agent:
        ddot:
          url: yaml-ddot-registry.example.com
          auth: yaml-ddot-auth
          username: yaml-ddot-user
          password: yaml-ddot-pass
        other-ext:
          url: yaml-other-ext-registry.example.com
          auth: yaml-other-ext-auth
          username: yaml-other-ext-user
          password: yaml-other-ext-pass
`,
			expected: &Env{
				APIKey:               "yaml-api-key",
				Site:                 "yaml-site.example.com",
				RegistryOverride:     "yaml-registry.example.com",
				RegistryAuthOverride: "yaml-auth",
				RegistryUsername:     "yaml-user",
				RegistryPassword:     "yaml-pass",

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
				Tags: []string{},

				ExtensionRegistryOverrides: map[string]map[string]ExtensionRegistryOverride{
					"datadog-agent": {
						"ddot": {
							URL:      "yaml-ddot-registry.example.com",
							Auth:     "yaml-ddot-auth",
							Username: "yaml-ddot-user",
							Password: "yaml-ddot-pass",
						},
						"other-ext": {
							URL:      "yaml-other-ext-registry.example.com",
							Auth:     "yaml-other-ext-auth",
							Username: "yaml-other-ext-user",
							Password: "yaml-other-ext-pass",
						},
					},
				},
			},
		},
		{
			name: "env vars take precedence over config",
			yaml: `
api_key: yaml-api-key
site: yaml-site.example.com
installer:
  registry:
    url: yaml-registry.example.com
    auth: yaml-auth
    username: yaml-user
    password: yaml-pass
`,
			envVars: map[string]string{
				"DD_API_KEY":                 "env-api-key",
				"DD_SITE":                    "env-site.example.com",
				"DD_INSTALLER_REGISTRY_URL":  "env-registry.example.com",
				"DD_INSTALLER_REGISTRY_AUTH": "env-auth",
			},
			expected: &Env{
				APIKey:                         "env-api-key",
				Site:                           "env-site.example.com",
				RegistryOverride:               "env-registry.example.com",
				RegistryAuthOverride:           "env-auth",
				RegistryUsername:               "yaml-user",
				RegistryPassword:               "yaml-pass",
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
				ExtensionRegistryOverrides: map[string]map[string]ExtensionRegistryOverride{},
			},
		},
		{
			name: "APM libraries parsing",
			envVars: map[string]string{
				envApmLibraries: "java  dotnet:latest, ruby:1.2   ,python:1.2.3",
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
				ExtensionRegistryOverrides: map[string]map[string]ExtensionRegistryOverride{},
			},
		},
		{
			name: "invalid yaml",
			envVars: map[string]string{
				"DD_API_KEY": "env-api-key",
			},
			yaml: `
{{invalid yaml
api_key: yaml-api-key
`,
			expected: &Env{
				APIKey:                         "env-api-key",
				Site:                           "datadoghq.com",
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
				ExtensionRegistryOverrides: map[string]map[string]ExtensionRegistryOverride{},
			},
		},
		{
			name:    "partial config fills only provided fields",
			envVars: map[string]string{},
			yaml: `
installer:
  registry:
    auth: yaml-auth
`,
			expected: &Env{
				Site:                           "datadoghq.com",
				RegistryAuthOverride:           "yaml-auth",
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
				ExtensionRegistryOverrides: map[string]map[string]ExtensionRegistryOverride{},
			},
		},
		{
			name: "registry URL from YAML applied without auth (bug fix)",
			yaml: `
installer:
  registry:
    url: yaml-registry.example.com
`,
			expected: &Env{
				RegistryOverride:               "yaml-registry.example.com",
				RegistryAuthOverride:           "",
				Site:                           "datadoghq.com",
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
				ExtensionRegistryOverrides: map[string]map[string]ExtensionRegistryOverride{},
			},
		},
		{
			name: "config site overrides default when DD_SITE not set",
			yaml: `
site: custom-site.example.com
`,
			expected: &Env{
				Site:                           "custom-site.example.com",
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
				ExtensionRegistryOverrides: map[string]map[string]ExtensionRegistryOverride{},
			},
		},
		{
			name: "DD_SITE explicitly set to default value beats config",
			yaml: `
site: custom-site.example.com
`,
			envVars: map[string]string{
				"DD_SITE": "datadoghq.com",
			},
			expected: &Env{
				Site:                           "datadoghq.com",
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
				ExtensionRegistryOverrides: map[string]map[string]ExtensionRegistryOverride{},
			},
		},
		{
			name: "primary set",
			envVars: map[string]string{
				"DD_AGENT_USER_NAME": "customuser",
			},
			expected: &Env{
				MsiParams: MsiParamsEnv{
					AgentUserName: "customuser",
				},
				Site:                           "datadoghq.com",
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
				ExtensionRegistryOverrides: map[string]map[string]ExtensionRegistryOverride{},
			},
		},
		{
			name: "compat set",
			envVars: map[string]string{
				"DDAGENTUSER_NAME": "customuser",
			},
			expected: &Env{
				MsiParams: MsiParamsEnv{
					AgentUserName: "customuser",
				},
				Site:                           "datadoghq.com",
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
				MsiParams: MsiParamsEnv{
					AgentUserName: "customuser",
				},
				Site:                           "datadoghq.com",
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
			dir := t.TempDir()

			if tc.yaml != "" {
				writeDatadogYAML(t, dir, tc.yaml)
			}
			for k, v := range tc.envVars {
				t.Setenv(k, v)
			}

			env := Get(WithConfigDir(dir))

			assert.Equal(t, tc.expected, env)
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
