// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
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
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "datadog.yaml"), []byte(configContent), 0644))

	e := env.Get(env.WithConfigDir(dir))

	assert.Equal(t, "default.registry.com", e.RegistryOverride)
	assert.Equal(t, "password", e.RegistryAuthOverride)
	assert.Equal(t, "defaultuser", e.RegistryUsername)
	assert.Equal(t, "defaultpass", e.RegistryPassword)

	require.Contains(t, e.ExtensionRegistryOverrides, agentPackage)
	agentExts := e.ExtensionRegistryOverrides[agentPackage]
	require.Len(t, agentExts, 2)

	ddot := agentExts["ddot"]
	assert.Equal(t, "custom.registry.com", ddot.URL)
	assert.Equal(t, "password", ddot.Auth)
	assert.Equal(t, "customuser", ddot.Username)
	assert.Equal(t, "custompass", ddot.Password)

	other := agentExts["other-ext"]
	assert.Equal(t, "other.registry.com", other.URL)
	assert.Empty(t, other.Auth)
}

func TestParseRegistryConfigNoExtensions(t *testing.T) {
	configContent := `
installer:
  registry:
    url: default.registry.com
`
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "datadog.yaml"), []byte(configContent), 0644))

	e := env.Get(env.WithConfigDir(dir))

	assert.Equal(t, "default.registry.com", e.RegistryOverride)
	assert.Empty(t, e.ExtensionRegistryOverrides)
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
