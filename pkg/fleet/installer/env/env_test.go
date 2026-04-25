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
				DefaultPackagesInstallOverride: map[string]bool{},
				DefaultPackagesVersionOverride: map[string]string{},
				ApmLibraries:                   map[ApmLibLanguage]ApmLibVersion{},
				InstallScript: InstallScriptEnv{
					APMInstrumentationEnabled: APMInstrumentationNotSet,
				},
				Tags:       []string{},
				Hostname:   "",
				HTTPProxy:  "",
				HTTPSProxy: "",
				NoProxy:    os.Getenv("NO_PROXY"), // Default value from the environment, as some test envs set it
			},
		},
		{
			name: "DD_INSTALLER_REGISTRY JSON populates Registry",
			envVars: map[string]string{
				EnvInstallerRegistry: `{
					"default": {"url": "registry.example.com", "auth": "auth", "username": "u", "password": "p"},
					"packages": {
						"datadog-agent": {"url": "agent.example.com", "extensions": {"ddot": {"auth": "password", "username": "cu", "password": "cp"}}}
					}
				}`,
			},
			expected: &Env{
				APIKey: "",
				Site:   "datadoghq.com",
				Registry: RegistryConfig{
					Default: RegistryEntry{URL: "registry.example.com", Auth: "auth", Username: "u", Password: "p"},
					Packages: map[string]PackageRegistry{
						"datadog-agent": {
							RegistryEntry: RegistryEntry{URL: "agent.example.com"},
							Extensions: map[string]RegistryEntry{
								"ddot": {Auth: "password", Username: "cu", Password: "cp"},
							},
						},
					},
				},
				DefaultPackagesInstallOverride: map[string]bool{},
				DefaultPackagesVersionOverride: map[string]string{},
				ApmLibraries:                   map[ApmLibLanguage]ApmLibVersion{},
				InstallScript: InstallScriptEnv{
					APMInstrumentationEnabled: APMInstrumentationNotSet,
				},
				Tags:    []string{},
				NoProxy: os.Getenv("NO_PROXY"),
			},
		},
		{
			name: "All environment variables set (except Registry, which is JSON)",
			envVars: map[string]string{
				envAPIKey:                             "123456",
				envSite:                               "datadoghq.eu",
				envRemoteUpdates:                      "true",
				envMirror:                             "https://mirror.example.com",
				envDefaultPackageInstall + "_PACKAGE": "true",
				envDefaultPackageInstall + "_ANOTHER_PACKAGE": "false",
				envDefaultPackageVersion + "_PACKAGE":         "1.2.3",
				envDefaultPackageVersion + "_ANOTHER_PACKAGE": "4.5.6",
				envApmLibraries:              "java,dotnet:latest,ruby:1.2",
				envApmInstrumentationEnabled: "all",
				envAgentUserName:             "customuser",
				envTags:                      "k1:v1,k2:v2",
				envExtraTags:                 "k3:v3,k4:v4",
				envHostname:                  "hostname",
				envDDHTTPProxy:               "http://proxy.example.com:8080",
				envDDHTTPSProxy:              "http://proxy.example.com:8080",
				envDDNoProxy:                 "localhost",
				envInfrastructureMode:        "basic",
				envAppKey:                    "app_key_123",
				envPAREnabled:                "true",
				envPARActionsAllowlist:       "com.datadoghq.script.runPredefinedScript,com.datadoghq.script.testConnection",
			},
			expected: &Env{
				APIKey:        "123456",
				Site:          "datadoghq.eu",
				Mirror:        "https://mirror.example.com",
				RemoteUpdates: true,
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
				Tags:                []string{"k1:v1", "k2:v2", "k3:v3", "k4:v4"},
				Hostname:            "hostname",
				HTTPProxy:           "http://proxy.example.com:8080",
				HTTPSProxy:          "http://proxy.example.com:8080",
				NoProxy:             "localhost",
				InfrastructureMode:  "basic",
				AppKey:              "app_key_123",
				PAREnabled:          true,
				PARActionsAllowlist: "com.datadoghq.script.runPredefinedScript,com.datadoghq.script.testConnection",
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
				Tags:    []string{},
				NoProxy: os.Getenv("NO_PROXY"),
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
				},
				DefaultPackagesInstallOverride: map[string]bool{},
				DefaultPackagesVersionOverride: map[string]string{},
				Tags:                           []string{},
				Hostname:                       "",
				NoProxy:                        os.Getenv("NO_PROXY"),
			},
		},
	}

	// Unset DD_API_KEY before the test matrix runs — some CI environments
	// set it, which would leak into FromEnv() results and break assertions.
	t.Setenv(envAPIKey, "")
	os.Unsetenv(envAPIKey)
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
		name             string
		env              *Env
		expectedContains []string
		expectedExcludes []string
	}{
		{
			name:             "Empty configuration",
			env:              &Env{},
			expectedContains: nil,
		},
		{
			name: "Registry field emitted as single DD_INSTALLER_REGISTRY JSON var",
			env: &Env{
				APIKey: "123456",
				Registry: RegistryConfig{
					Default: RegistryEntry{URL: "registry.example.com"},
					Packages: map[string]PackageRegistry{
						"datadog-agent": {RegistryEntry: RegistryEntry{URL: "agent.example.com"}},
					},
				},
			},
			expectedContains: []string{
				"DD_API_KEY=123456",
				`DD_INSTALLER_REGISTRY={"default":{"url":"registry.example.com"},"packages":{"datadog-agent":{"url":"agent.example.com"}}}`,
			},
		},
		{
			name: "Empty Registry does not emit DD_INSTALLER_REGISTRY",
			env: &Env{
				APIKey: "123456",
			},
			expectedContains: []string{"DD_API_KEY=123456"},
			expectedExcludes: []string{"DD_INSTALLER_REGISTRY="},
		},
		{
			name: "All configuration set",
			env: &Env{
				APIKey:        "123456",
				Site:          "datadoghq.eu",
				RemoteUpdates: true,
				Mirror:        "https://mirror.example.com",
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
			expectedContains: []string{
				"DD_API_KEY=123456",
				"DD_SITE=datadoghq.eu",
				"DD_REMOTE_UPDATES=true",
				"DD_INSTALLER_MIRROR=https://mirror.example.com",
				"DD_APM_INSTRUMENTATION_LIBRARIES=dotnet:latest,java,ruby:1.2",
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
			expectedContains: []string{
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
			expectedContains: []string{"DD_API_KEY=123456"},
			expectedExcludes: []string{"DD_APP_KEY=", "DD_PRIVATE_ACTION_RUNNER_"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.env.ToEnv()
			for _, want := range tt.expectedContains {
				assert.Contains(t, result, want, "expected env contains %q", want)
			}
			for _, excludePrefix := range tt.expectedExcludes {
				for _, got := range result {
					assert.NotContains(t, got, excludePrefix, "unexpected env var with prefix %q", excludePrefix)
				}
			}
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
