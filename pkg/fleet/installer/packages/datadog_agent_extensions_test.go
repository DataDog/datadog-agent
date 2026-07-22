// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	extensionsPkg "github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/extensions"
)

func TestParseRegistryConfigExtensionOverrides(t *testing.T) {
	configContent := `
installer:
  registry:
    url: default.registry.com
    auth: password
    username: defaultuser
    password: defaultpass
    extensions:
      datadog-agent:
        ddot:
          url: custom.registry.com
          auth: password
          username: customuser
          password: custompass
        other-ext:
          url: other.registry.com
`
	var config datadogAgentConfig
	err := yaml.Unmarshal([]byte(configContent), &config)
	require.NoError(t, err)

	assert.Equal(t, "default.registry.com", config.Installer.Registry.URL)
	assert.Equal(t, "password", config.Installer.Registry.Auth)
	assert.Equal(t, "defaultuser", config.Installer.Registry.Username)
	assert.Equal(t, "defaultpass", config.Installer.Registry.Password)

	require.Contains(t, config.Installer.Registry.Extensions, agentPackage)
	agentExts := config.Installer.Registry.Extensions[agentPackage]
	require.Len(t, agentExts, 2)

	ddot := agentExts["ddot"]
	assert.Equal(t, "custom.registry.com", ddot.URL)
	assert.Equal(t, "password", ddot.Auth)
	assert.Equal(t, "customuser", ddot.Username)
	assert.Equal(t, "custompass", ddot.Password)

	other := agentExts["other-ext"]
	assert.Equal(t, "other.registry.com", other.URL)
	assert.Empty(t, other.Auth)

	// Verify conversion to ExtensionRegistry overrides map
	overrides := make(map[string]extensionsPkg.ExtensionRegistry, len(agentExts))
	for extName, extCfg := range agentExts {
		overrides[extName] = extensionsPkg.ExtensionRegistry{
			URL:      extCfg.URL,
			Auth:     extCfg.Auth,
			Username: extCfg.Username,
			Password: extCfg.Password,
		}
	}
	require.Len(t, overrides, 2)
	assert.Equal(t, "custom.registry.com", overrides["ddot"].URL)
	assert.Equal(t, "other.registry.com", overrides["other-ext"].URL)
}

func TestParseRegistryConfigNoExtensions(t *testing.T) {
	configContent := `
installer:
  registry:
    url: default.registry.com
`
	var config datadogAgentConfig
	err := yaml.Unmarshal([]byte(configContent), &config)
	require.NoError(t, err)

	assert.Equal(t, "default.registry.com", config.Installer.Registry.URL)
	assert.Nil(t, config.Installer.Registry.Extensions)
}

func TestEndUserDeviceModeEnabled(t *testing.T) {
	const noConfigRead = "<config-should-not-be-read>"
	tests := []struct {
		name       string
		envMode    string // DD_INFRASTRUCTURE_MODE
		configMode string // datadog.yaml infrastructure_mode; noConfigRead asserts it is not consulted
		want       bool
	}{
		{name: "env end_user_device enables", envMode: "end_user_device", configMode: noConfigRead, want: true},
		{name: "env case-insensitive", envMode: "End_User_Device", configMode: noConfigRead, want: true},
		// env is authoritative when set: a non-EUDM env value disables EUDM without even reading
		// the config, even though the config still says end_user_device.
		{name: "env full overrides config end_user_device", envMode: "full", configMode: noConfigRead, want: false},
		{name: "env other value disables", envMode: "basic", configMode: noConfigRead, want: false},
		// config is only consulted when the env var is blank.
		{name: "blank env falls back to config end_user_device", envMode: "", configMode: "end_user_device", want: true},
		{name: "blank env falls back to config full", envMode: "", configMode: "full", want: false},
		{name: "blank env and empty config", envMode: "", configMode: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configModeFn := func() string {
				if tt.configMode == noConfigRead {
					t.Fatalf("config should not be read when DD_INFRASTRUCTURE_MODE is set")
				}
				return tt.configMode
			}
			assert.Equal(t, tt.want, endUserDeviceModeEnabled(tt.envMode, configModeFn))
		})
	}
}

func TestInstallDDOTExtensionIfEnabled_Disabled(t *testing.T) {
	t.Setenv("DD_OTELCOLLECTOR_ENABLED", "false")
	ctx := HookContext{Context: context.Background()}
	err := installAgentExtensions(ctx, "7.50.0-1", false)
	require.NoError(t, err)
}

func TestInstallDDOTExtensionIfEnabled_Enabled(t *testing.T) {
	t.Setenv("DD_OTELCOLLECTOR_ENABLED", "true")

	tmpDir := t.TempDir()
	extensionsPkg.ExtensionsDBDir = tmpDir

	ctx := HookContext{Context: context.Background(), PackagePath: tmpDir}
	err := extensionsPkg.SetPackage(ctx, agentPackage, "7.50.0-1", false)
	require.NoError(t, err)

	err = installAgentExtensions(ctx, "7.50.0-1", false)
	// Expect a download error (no real OCI registry), not an env-guard skip
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "otelcollector")
}
