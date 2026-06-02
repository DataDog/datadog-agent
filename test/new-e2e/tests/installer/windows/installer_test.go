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

// --- NewPackageConfig ---

func TestNewPackageConfig(t *testing.T) {
	tests := []struct {
		name    string
		opts    []PackageOption
		env     map[string]string
		check   func(*testing.T, TestPackageConfig)
		wantErr string
	}{
		// --- Release version ---
		{
			name: "release version via options",
			opts: []PackageOption{WithName(consts.AgentPackage), WithVersion("7.75.0-1")},
			check: func(t *testing.T, c TestPackageConfig) {
				assert.Equal(t, consts.StableS3OCIRegistry, c.Registry)
			},
		},
		{
			name: "SOURCE_VERSION env clears pipeline from prior option",
			opts: []PackageOption{WithName(consts.AgentPackage), WithPipeline("99999"), WithArtifactOverrides("TEST")},
			env:  map[string]string{"TEST_SOURCE_VERSION": "7.75.0-1"},
			check: func(t *testing.T, c TestPackageConfig) {
				assert.Equal(t, "7.75.0-1", c.Version)
				assert.Equal(t, consts.StableS3OCIRegistry, c.Registry, "Registry should be re-inferred by Resolve")
			},
		},

		// --- RC version ---
		{
			name: "RC version via options",
			opts: []PackageOption{WithName(consts.AgentPackage), WithVersion("7.76.0-rc.2-1")},
			check: func(t *testing.T, c TestPackageConfig) {
				assert.Equal(t, "7.76.0-rc.2-1", c.Version)
				assert.Equal(t, consts.BetaS3OCIRegistry, c.Registry)
			},
		},
		{
			name: "RC version via SOURCE_VERSION env",
			opts: []PackageOption{WithName(consts.AgentPackage), WithArtifactOverrides("TEST")},
			env:  map[string]string{"TEST_SOURCE_VERSION": "7.76.0-rc.2-1"},
			check: func(t *testing.T, c TestPackageConfig) {
				assert.Equal(t, "7.76.0-rc.2-1", c.Version)
				assert.Equal(t, consts.BetaS3OCIRegistry, c.Registry)
			},
		},

		// --- Pipeline version ---
		{
			name: "pipeline via options",
			opts: []PackageOption{WithName(consts.AgentPackage), WithPipeline("12345")},
			check: func(t *testing.T, c TestPackageConfig) {
				assert.Equal(t, consts.AgentPackage, c.Name)
				assert.Equal(t, "agent-package", c.Alias)
				assert.Equal(t, "pipeline-12345", c.Version)
				assert.Equal(t, consts.PipelineOCIRegistry, c.Registry)
			},
		},
		{
			name: "pipeline via PIPELINE env with auth",
			opts: []PackageOption{WithName(consts.AgentPackage), WithArtifactOverrides("TEST")},
			env:  map[string]string{"TEST_PIPELINE": "12345", "TEST_OCI_AUTH": "my-token"},
			check: func(t *testing.T, c TestPackageConfig) {
				assert.Equal(t, "pipeline-12345", c.Version)
				assert.Equal(t, consts.PipelineOCIRegistry, c.Registry)
				assert.Equal(t, "my-token", c.Auth)
			},
		},

		// --- Override trumps option ---
		{
			name: "env pipeline override replaces option-defined release",
			opts: []PackageOption{WithName(consts.AgentPackage), WithVersion("7.75.0-1"), WithArtifactOverrides("TEST")},
			env:  map[string]string{"TEST_PIPELINE": "55555"},
			check: func(t *testing.T, c TestPackageConfig) {
				assert.Equal(t, "pipeline-55555", c.Version, "env override should take priority over WithVersion")
				assert.Equal(t, consts.PipelineOCIRegistry, c.Registry)
			},
		},

		// --- Direct URL / custom registry (no resolution) ---
		{
			name: "URL override via options",
			opts: []PackageOption{WithName(consts.AgentPackage), WithURLOverride("file:///local/package.tar")},
			check: func(t *testing.T, c TestPackageConfig) {
				assert.Equal(t, "file:///local/package.tar", c.URL())
				assert.Empty(t, c.Registry, "Registry should not be set when urloverride is present")
			},
		},
		{
			name: "custom registry skips resolve",
			opts: []PackageOption{WithName(consts.AgentPackage), WithVersion("7.75.0-1"), WithRegistry("my-custom-registry.com")},
			check: func(t *testing.T, c TestPackageConfig) {
				assert.Equal(t, "my-custom-registry.com", c.Registry, "pre-set Registry should not be overridden by Resolve")
			},
		},

		// --- All OCI overrides ---
		{
			name: "all OCI field overrides via env",
			opts: []PackageOption{WithName(consts.AgentPackage), WithArtifactOverrides("TEST")},
			env: map[string]string{
				"TEST_OCI_URL":      "file:///override/package.tar",
				"TEST_OCI_PIPELINE": "99999",
				"TEST_OCI_VERSION":  "custom-tag",
				"TEST_OCI_REGISTRY": "custom-registry.example.com",
				"TEST_OCI_AUTH":     "my-token",
			},
			check: func(t *testing.T, c TestPackageConfig) {
				assert.Equal(t, "file:///override/package.tar", c.URL())
				assert.Equal(t, "custom-tag", c.Version, "OCI_VERSION should override pipeline version")
				assert.Equal(t, "custom-registry.example.com", c.Registry, "OCI_REGISTRY should override pipeline registry")
				assert.Equal(t, "my-token", c.Auth)
			},
		},

		// --- Artifact overrides behavior ---
		{
			name: "artifact overrides with no env vars are no-op",
			opts: []PackageOption{WithName(consts.AgentPackage), WithVersion("7.75.0-1"), WithArtifactOverrides("TEST")},
			check: func(t *testing.T, c TestPackageConfig) {
				assert.Equal(t, "7.75.0-1", c.Version)
				assert.Equal(t, consts.StableS3OCIRegistry, c.Registry)
			},
		},
		{
			name: "artifact overrides apply even in CI",
			opts: []PackageOption{WithName(consts.AgentPackage), WithArtifactOverrides("TEST")},
			env:  map[string]string{"CI": "true", "TEST_SOURCE_VERSION": "7.75.0-1"},
			check: func(t *testing.T, c TestPackageConfig) {
				assert.Equal(t, "7.75.0-1", c.Version, "WithArtifactOverrides should apply even in CI")
			},
		},

		// --- Errors ---
		{
			name:    "SOURCE_VERSION and PIPELINE are mutually exclusive",
			opts:    []PackageOption{WithName(consts.AgentPackage), WithArtifactOverrides("TEST")},
			env:     map[string]string{"TEST_SOURCE_VERSION": "7.75.0-1", "TEST_PIPELINE": "12345"},
			wantErr: "mutually exclusive",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			cfg, err := NewPackageConfig(tc.opts...)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			tc.check(t, cfg)
		})
	}
}

// --- URL ---

func TestURL(t *testing.T) {
	tests := []struct {
		name string
		cfg  TestPackageConfig
		want string
	}{
		{
			name: "from parts with alias",
			cfg: TestPackageConfig{
				Name:     "datadog-agent",
				Alias:    "agent-package",
				Version:  "7.75.0-1",
				Registry: "dd-agent.s3.amazonaws.com",
			},
			want: "oci://dd-agent.s3.amazonaws.com/agent-package:7.75.0-1",
		},
		{
			name: "from parts without alias uses name",
			cfg: TestPackageConfig{
				Name:     "datadog-agent",
				Version:  "pipeline-12345",
				Registry: "installtesting.datad0g.com.internal.dda-testing.com",
			},
			want: "oci://installtesting.datad0g.com.internal.dda-testing.com/datadog-agent:pipeline-12345",
		},
		{
			name: "urloverride takes precedence",
			cfg: TestPackageConfig{
				Name:        "datadog-agent",
				Alias:       "agent-package",
				Version:     "7.75.0-1",
				Registry:    consts.StableS3OCIRegistry,
				urloverride: "file:///local/package.tar",
			},
			want: "file:///local/package.tar",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.cfg.URL())
		})
	}
}

// --- WithDevEnvOverrides ---

func TestOCIWithDevEnvOverrides(t *testing.T) {
	t.Run("applies overrides when not in CI", func(t *testing.T) {
		t.Setenv("CI", "")
		t.Setenv("DEV_SOURCE_VERSION", "7.75.0-1")

		c := &TestPackageConfig{}
		require.NoError(t, WithDevEnvOverrides("DEV")(c))
		assert.Equal(t, "7.75.0-1", c.Version)
	})

	t.Run("skips overrides in CI", func(t *testing.T) {
		t.Setenv("CI", "true")
		t.Setenv("DEV_SOURCE_VERSION", "7.75.0-1")
		t.Setenv("DEV_OCI_URL", "file:///local/package.tar")

		c := &TestPackageConfig{
			Version:  "pipeline-99999",
			Registry: consts.PipelineOCIRegistry,
		}
		require.NoError(t, WithDevEnvOverrides("DEV")(c))
		assert.Equal(t, "pipeline-99999", c.Version, "original version should be preserved in CI")
		assert.Equal(t, consts.PipelineOCIRegistry, c.Registry, "original registry should be preserved in CI")
		assert.Empty(t, c.urloverride, "env vars should not be applied in CI")
	})
}
