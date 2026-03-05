// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build e2eunit

package installer

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- PackageOption functions ---

func TestOCIPackageOptions(t *testing.T) {
	tests := []struct {
		name  string
		opt   PackageOption
		check func(*testing.T, *TestPackageConfig)
	}{
		{
			name: "WithName",
			opt:  WithName("datadog-agent"),
			check: func(t *testing.T, c *TestPackageConfig) {
				assert.Equal(t, "datadog-agent", c.Name)
			},
		},
		{
			name: "WithAlias",
			opt:  WithAlias("agent-package"),
			check: func(t *testing.T, c *TestPackageConfig) {
				assert.Equal(t, "agent-package", c.Alias)
			},
		},
		{
			name: "WithVersion",
			opt:  WithVersion("7.75.0-1"),
			check: func(t *testing.T, c *TestPackageConfig) {
				assert.Equal(t, "7.75.0-1", c.Version)
			},
		},
		{
			name: "WithRegistry",
			opt:  WithRegistry("my-registry.example.com"),
			check: func(t *testing.T, c *TestPackageConfig) {
				assert.Equal(t, "my-registry.example.com", c.Registry)
			},
		},
		{
			name: "WithAuthentication",
			opt:  WithAuthentication("token-abc"),
			check: func(t *testing.T, c *TestPackageConfig) {
				assert.Equal(t, "token-abc", c.Auth)
			},
		},
		{
			name: "WithURLOverride",
			opt:  WithURLOverride("file:///path/to/package.tar"),
			check: func(t *testing.T, c *TestPackageConfig) {
				assert.Equal(t, "file:///path/to/package.tar", c.urloverride)
			},
		},
		{
			name: "WithPipeline",
			opt:  WithPipeline("12345"),
			check: func(t *testing.T, c *TestPackageConfig) {
				assert.Equal(t, "pipeline-12345", c.Version)
				assert.Equal(t, consts.PipelineOCIRegistry, c.Registry)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &TestPackageConfig{}
			err := tc.opt(c)
			require.NoError(t, err)
			tc.check(t, c)
		})
	}
}

func TestWithPackageCopiesFields(t *testing.T) {
	src := TestPackageConfig{
		Name:     "datadog-agent",
		Alias:    "agent-package",
		Version:  "7.75.0-1",
		Registry: consts.StableS3OCIRegistry,
		Auth:     "token",
	}
	dst := &TestPackageConfig{}
	err := WithPackage(src)(dst)
	require.NoError(t, err)
	assert.Equal(t, src.Name, dst.Name)
	assert.Equal(t, src.Alias, dst.Alias)
	assert.Equal(t, src.Version, dst.Version)
	assert.Equal(t, src.Registry, dst.Registry)
	assert.Equal(t, src.Auth, dst.Auth)
}

// --- URL() ---

func TestURLFromParts(t *testing.T) {
	c := TestPackageConfig{
		Name:     "datadog-agent",
		Alias:    "agent-package",
		Version:  "7.75.0-1",
		Registry: consts.StableS3OCIRegistry,
	}
	assert.Equal(t, "oci://dd-agent.s3.amazonaws.com/agent-package:7.75.0-1", c.URL())
}

func TestURLFromPartsNoAlias(t *testing.T) {
	c := TestPackageConfig{
		Name:     "datadog-agent",
		Version:  "pipeline-12345",
		Registry: consts.PipelineOCIRegistry,
	}
	assert.Equal(t, "oci://installtesting.datad0g.com.internal.dda-testing.com/datadog-agent:pipeline-12345", c.URL())
}

func TestURLOverrideTakesPrecedence(t *testing.T) {
	c := TestPackageConfig{
		Name:        "datadog-agent",
		Alias:       "agent-package",
		Version:     "7.75.0-1",
		Registry:    consts.StableS3OCIRegistry,
		urloverride: "file:///local/package.tar",
	}
	assert.Equal(t, "file:///local/package.tar", c.URL())
}

// --- Resolve() ---

func TestResolveNoOpWhenURLOverrideSet(t *testing.T) {
	c := &TestPackageConfig{urloverride: "file:///local/package.tar"}
	require.NoError(t, c.Resolve())
	assert.Empty(t, c.Registry, "Registry should not be set when urloverride is present")
}

func TestResolveNoOpWhenRegistrySet(t *testing.T) {
	c := &TestPackageConfig{
		Registry: "custom-registry.example.com",
		Version:  "7.75.0-1",
	}
	require.NoError(t, c.Resolve())
	assert.Equal(t, "custom-registry.example.com", c.Registry, "pre-set Registry should not be overridden")
}

func TestResolveStableVersion(t *testing.T) {
	c := &TestPackageConfig{Version: "7.75.0-1"}
	require.NoError(t, c.Resolve())
	assert.Equal(t, consts.StableS3OCIRegistry, c.Registry)
}

func TestResolveRCVersion(t *testing.T) {
	c := &TestPackageConfig{Version: "7.76.0-rc.2-1"}
	require.NoError(t, c.Resolve())
	assert.Equal(t, consts.BetaS3OCIRegistry, c.Registry)
}

func TestResolveNothingSet(t *testing.T) {
	c := &TestPackageConfig{}
	require.NoError(t, c.Resolve())
	assert.Empty(t, c.Registry, "Registry should remain empty when nothing is set")
}

func TestResolvePipelineRegistryPreserved(t *testing.T) {
	c := &TestPackageConfig{
		Version:  "pipeline-12345",
		Registry: consts.PipelineOCIRegistry,
	}
	require.NoError(t, c.Resolve())
	assert.Equal(t, consts.PipelineOCIRegistry, c.Registry, "pipeline registry should be preserved")
}

// --- applyOCIEnvOverrides ---

func TestApplyOCIEnvOverridesSourceVersion(t *testing.T) {
	t.Setenv("TEST_SOURCE_VERSION", "7.75.0-1")

	c := &TestPackageConfig{Registry: consts.PipelineOCIRegistry}
	err := applyOCIEnvOverrides("TEST", c)
	require.NoError(t, err)
	assert.Equal(t, "7.75.0-1", c.Version)
	assert.Empty(t, c.Registry, "Registry should be cleared for fresh inference by Resolve")
}

func TestApplyOCIEnvOverridesPipeline(t *testing.T) {
	t.Setenv("TEST_PIPELINE", "12345")

	c := &TestPackageConfig{}
	err := applyOCIEnvOverrides("TEST", c)
	require.NoError(t, err)
	assert.Equal(t, "pipeline-12345", c.Version)
	assert.Equal(t, consts.PipelineOCIRegistry, c.Registry)
}

func TestApplyOCIEnvOverridesMutualExclusion(t *testing.T) {
	t.Setenv("TEST_SOURCE_VERSION", "7.75.0-1")
	t.Setenv("TEST_PIPELINE", "12345")

	c := &TestPackageConfig{}
	err := applyOCIEnvOverrides("TEST", c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestApplyOCIEnvOverridesOCIOverrides(t *testing.T) {
	t.Setenv("TEST_OCI_URL", "file:///override/package.tar")
	t.Setenv("TEST_OCI_PIPELINE", "99999")
	t.Setenv("TEST_OCI_VERSION", "custom-tag")
	t.Setenv("TEST_OCI_REGISTRY", "custom-registry.example.com")
	t.Setenv("TEST_OCI_AUTH", "my-token")

	c := &TestPackageConfig{}
	err := applyOCIEnvOverrides("TEST", c)
	require.NoError(t, err)
	assert.Equal(t, "file:///override/package.tar", c.urloverride)
	assert.Equal(t, "custom-tag", c.Version, "OCI_VERSION should override pipeline version")
	assert.Equal(t, "custom-registry.example.com", c.Registry, "OCI_REGISTRY should override pipeline registry")
	assert.Equal(t, "my-token", c.Auth)
}

func TestApplyOCIEnvOverridesOCIPipelineOverridesSourceVersion(t *testing.T) {
	t.Setenv("TEST_SOURCE_VERSION", "7.75.0-1")
	t.Setenv("TEST_OCI_PIPELINE", "99999")

	c := &TestPackageConfig{}
	err := applyOCIEnvOverrides("TEST", c)
	require.NoError(t, err)
	assert.Equal(t, "pipeline-99999", c.Version, "OCI_PIPELINE should override SOURCE_VERSION")
	assert.Equal(t, consts.PipelineOCIRegistry, c.Registry)
}

func TestApplyOCIEnvOverridesVersionOverridesPipeline(t *testing.T) {
	t.Setenv("TEST_PIPELINE", "12345")
	t.Setenv("TEST_OCI_VERSION", "my-custom-tag")

	c := &TestPackageConfig{}
	err := applyOCIEnvOverrides("TEST", c)
	require.NoError(t, err)
	assert.Equal(t, "my-custom-tag", c.Version, "OCI_VERSION should override PIPELINE version")
	assert.Equal(t, consts.PipelineOCIRegistry, c.Registry, "PIPELINE registry should still be set")
}

func TestApplyOCIEnvOverridesNoVarsSet(t *testing.T) {
	c := &TestPackageConfig{
		Version:  "existing-version",
		Registry: "existing-registry",
	}
	err := applyOCIEnvOverrides("TEST", c)
	require.NoError(t, err)
	assert.Equal(t, "existing-version", c.Version, "fields should not change when no env vars are set")
	assert.Equal(t, "existing-registry", c.Registry, "fields should not change when no env vars are set")
}

// --- WithArtifactOverrides / WithDevEnvOverrides ---

func TestOCIWithArtifactOverrides(t *testing.T) {
	t.Setenv("MYPREFIX_PIPELINE", "12345")
	t.Setenv("MYPREFIX_OCI_AUTH", "my-token")

	c := &TestPackageConfig{}
	err := WithArtifactOverrides("MYPREFIX")(c)
	require.NoError(t, err)
	assert.Equal(t, "pipeline-12345", c.Version)
	assert.Equal(t, consts.PipelineOCIRegistry, c.Registry)
	assert.Equal(t, "my-token", c.Auth)
}

func TestOCIWithArtifactOverridesAlwaysApplied(t *testing.T) {
	t.Setenv("CI", "true")
	t.Setenv("MYPREFIX_SOURCE_VERSION", "7.75.0-1")

	c := &TestPackageConfig{}
	err := WithArtifactOverrides("MYPREFIX")(c)
	require.NoError(t, err)
	assert.Equal(t, "7.75.0-1", c.Version, "WithArtifactOverrides should apply even in CI")
}

func TestOCIWithDevEnvOverridesNotInCI(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("DEV_SOURCE_VERSION", "7.75.0-1")

	c := &TestPackageConfig{}
	err := WithDevEnvOverrides("DEV")(c)
	require.NoError(t, err)
	assert.Equal(t, "7.75.0-1", c.Version)
}

func TestOCIWithDevEnvOverridesInCI(t *testing.T) {
	t.Setenv("CI", "true")
	t.Setenv("DEV_SOURCE_VERSION", "7.75.0-1")
	t.Setenv("DEV_OCI_URL", "file:///local/package.tar")

	c := &TestPackageConfig{
		Version:  "pipeline-99999",
		Registry: consts.PipelineOCIRegistry,
	}
	err := WithDevEnvOverrides("DEV")(c)
	require.NoError(t, err)
	assert.Equal(t, "pipeline-99999", c.Version, "original version should be preserved in CI")
	assert.Equal(t, consts.PipelineOCIRegistry, c.Registry, "original registry should be preserved in CI")
	assert.Empty(t, c.urloverride, "env vars should not be applied in CI")
}

// --- NewPackageConfig integration ---

func TestNewPackageConfigWithPipeline(t *testing.T) {
	cfg, err := NewPackageConfig(
		WithName(consts.AgentPackage),
		WithPipeline("12345"),
	)
	require.NoError(t, err)
	assert.Equal(t, consts.AgentPackage, cfg.Name)
	assert.Equal(t, "agent-package", cfg.Alias)
	assert.Equal(t, "pipeline-12345", cfg.Version)
	assert.Equal(t, consts.PipelineOCIRegistry, cfg.Registry)
	assert.Equal(t, "oci://installtesting.datad0g.com.internal.dda-testing.com/agent-package:pipeline-12345", cfg.URL())
}

func TestNewPackageConfigWithVersion(t *testing.T) {
	cfg, err := NewPackageConfig(
		WithName(consts.AgentPackage),
		WithVersion("7.75.0-1"),
	)
	require.NoError(t, err)
	assert.Equal(t, consts.StableS3OCIRegistry, cfg.Registry, "stable version should resolve to stable registry")
}

func TestNewPackageConfigWithRCVersion(t *testing.T) {
	cfg, err := NewPackageConfig(
		WithName(consts.AgentPackage),
		WithVersion("7.76.0-rc.2-1"),
	)
	require.NoError(t, err)
	assert.Equal(t, consts.BetaS3OCIRegistry, cfg.Registry, "RC version should resolve to beta registry")
}

func TestNewPackageConfigWithURLOverride(t *testing.T) {
	cfg, err := NewPackageConfig(
		WithName(consts.AgentPackage),
		WithURLOverride("file:///local/package.tar"),
	)
	require.NoError(t, err)
	assert.Equal(t, "file:///local/package.tar", cfg.URL())
	assert.Empty(t, cfg.Registry, "Registry should not be set when urloverride is present")
}

func TestNewPackageConfigWithRegistrySkipsResolve(t *testing.T) {
	cfg, err := NewPackageConfig(
		WithName(consts.AgentPackage),
		WithVersion("7.75.0-1"),
		WithRegistry("my-custom-registry.com"),
	)
	require.NoError(t, err)
	assert.Equal(t, "my-custom-registry.com", cfg.Registry, "pre-set Registry should not be overridden by Resolve")
}

func TestNewPackageConfigWithArtifactOverrides(t *testing.T) {
	t.Setenv("TEST_PIPELINE", "55555")

	cfg, err := NewPackageConfig(
		WithName(consts.AgentPackage),
		WithVersion("7.75.0-1"),
		WithArtifactOverrides("TEST"),
	)
	require.NoError(t, err)
	assert.Equal(t, "pipeline-55555", cfg.Version, "env override should take priority over WithVersion")
	assert.Equal(t, consts.PipelineOCIRegistry, cfg.Registry)
}
