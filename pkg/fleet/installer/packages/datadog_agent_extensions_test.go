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

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	extensionsPkg "github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/extensions"
)

func TestExtensionOverrides(t *testing.T) {
	e := &env.Env{
		Registry: env.RegistryConfig{
			Default: env.RegistryEntry{URL: "default.registry.com", Auth: "password", Username: "defaultuser", Password: "defaultpass"},
			Packages: map[string]env.PackageRegistry{
				agentPackage: {
					Extensions: map[string]env.RegistryEntry{
						"ddot": {
							URL:      "custom.registry.com",
							Auth:     "password",
							Username: "customuser",
							Password: "custompass",
						},
						"other-ext": {URL: "other.registry.com"},
					},
				},
			},
		},
	}

	got := extensionOverrides(e, []string{"ddot", "other-ext"})
	require.Len(t, got, 2)
	assert.Equal(t, extensionsPkg.ExtensionRegistry{
		URL: "custom.registry.com", Auth: "password", Username: "customuser", Password: "custompass",
	}, got["ddot"])
	assert.Equal(t, extensionsPkg.ExtensionRegistry{URL: "other.registry.com"}, got["other-ext"])
}

func TestExtensionOverridesEmpty(t *testing.T) {
	e := &env.Env{
		Registry: env.RegistryConfig{
			Default: env.RegistryEntry{URL: "default.registry.com"},
		},
	}
	assert.Nil(t, extensionOverrides(e, []string{"ddot"}))
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
