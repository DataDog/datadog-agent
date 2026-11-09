// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || windows || darwin

package preflightmodeimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestIsEligible(t *testing.T) {
	tests := []struct {
		name string
		// env is applied before the config is built, so it lands in the env var layer.
		env map[string]string
		// setup mutates the config before the assertion; nil means "leave defaults".
		setup func(cfg pkgconfigmodel.Config)
		want  bool
		// wantReason, when set, must appear in the reason returned alongside a false result.
		wantReason string
	}{
		{
			name: "defaults are eligible",
			want: true,
		},
		{
			name: "preflight_mode explicitly disabled",
			setup: func(cfg pkgconfigmodel.Config) {
				cfg.Set(DataPlanePreflightMode, false, pkgconfigmodel.SourceFile)
			},
			want:       false,
			wantReason: DataPlanePreflightMode,
		},
		{
			name: "data_plane.enabled explicitly true means ADP already runs for real",
			setup: func(cfg pkgconfigmodel.Config) {
				cfg.Set(DataPlaneEnabled, true, pkgconfigmodel.SourceFile)
			},
			want:       false,
			wantReason: DataPlaneEnabled,
		},
		{
			name: "data_plane.enabled explicitly false means the operator opted out",
			setup: func(cfg pkgconfigmodel.Config) {
				cfg.Set(DataPlaneEnabled, false, pkgconfigmodel.SourceFile)
			},
			want:       false,
			wantReason: DataPlaneEnabled,
		},
		{
			name: "data_plane.enabled set by env is still explicit",
			env:  map[string]string{"DD_DATA_PLANE_ENABLED": "true"},
			want: false,
		},
		{
			name: "preflight_mode disabled by env",
			env:  map[string]string{"DD_DATA_PLANE_PREFLIGHT_MODE": "false"},
			want: false,
		},
		{
			name: "data_plane.enabled set by fleet policies is still explicit",
			setup: func(cfg pkgconfigmodel.Config) {
				cfg.Set(DataPlaneEnabled, false, pkgconfigmodel.SourceFleetPolicies)
			},
			want: false,
		},
		{
			// This is what the platform gate in pkg/config/setup installs on platforms where
			// ADP cannot run, and on Windows without process_manager.enabled. Expressed as the
			// resulting override rather than by calling that gate, which is unexported there;
			// that it produces a SourceAgentRuntime override is asserted by
			// TestSanitizeDataPlaneConfig in pkg/config/setup.
			name: "a SourceAgentRuntime lock makes it ineligible",
			setup: func(cfg pkgconfigmodel.Config) {
				cfg.Set(DataPlaneEnabled, false, pkgconfigmodel.SourceAgentRuntime)
			},
			want: false,
		},
		{
			// The remaining cases pin the secrets gate: the pre-flight writes the resolved
			// configuration to disk, so it must not run when a secret could be in it.
			name: "secret_backend_command set",
			setup: func(cfg pkgconfigmodel.Config) {
				cfg.Set(secretBackendCommand, "/usr/local/bin/fetch-secrets", pkgconfigmodel.SourceFile)
			},
			want:       false,
			wantReason: secretBackendCommand,
		},
		{
			name: "secret_backend_type set",
			setup: func(cfg pkgconfigmodel.Config) {
				cfg.Set(secretBackendType, "aws.secrets", pkgconfigmodel.SourceFile)
			},
			want:       false,
			wantReason: secretBackendType,
		},
		{
			name: "secret_backend_command set by env",
			env:  map[string]string{"DD_SECRET_BACKEND_COMMAND": "/usr/local/bin/fetch-secrets"},
			want: false,
		},
		{
			name: "secret_backend_type set by env",
			env:  map[string]string{"DD_SECRET_BACKEND_TYPE": "file.yaml"},
			want: false,
		},
		{
			name: "multi_secret_backends set",
			setup: func(cfg pkgconfigmodel.Config) {
				cfg.Set(multiSecretBackends, map[string]interface{}{
					"vault": map[string]interface{}{"type": "hashicorp.vault"},
				}, pkgconfigmodel.SourceFile)
			},
			want:       false,
			wantReason: multiSecretBackends,
		},
		{
			name: "an empty secret backend does not gate anything",
			setup: func(cfg pkgconfigmodel.Config) {
				cfg.Set(secretBackendCommand, "", pkgconfigmodel.SourceFile)
				cfg.Set(secretBackendType, "", pkgconfigmodel.SourceFile)
				cfg.Set(multiSecretBackends, map[string]interface{}{}, pkgconfigmodel.SourceFile)
			},
			want: true,
		},
		{
			// A resolver that already ran leaves its output in the secrets layer. This is the
			// case that matters most: the value is only in memory today, and the pre-flight
			// would be what puts it on disk.
			name: "a value in the secrets layer",
			setup: func(cfg pkgconfigmodel.Config) {
				cfg.Set("api_key", "resolved-api-key", pkgconfigmodel.SourceSecret)
			},
			want:       false,
			wantReason: "api_key",
		},
		{
			// The backend settings would not catch this: they can be cleared from the config
			// (or the handles can live somewhere other than datadog.yaml) while a resolved
			// value is still sitting in the layer.
			name: "a value in the secrets layer with no backend configured",
			setup: func(cfg pkgconfigmodel.Config) {
				cfg.Set("proxy.https", "https://user:resolved-password@proxy:3128", pkgconfigmodel.SourceSecret)
			},
			want:       false,
			wantReason: "proxy.https",
		},
		{
			// Secrets inside a compound setting are assigned wholesale at the nearest known
			// key, so the walk has to see them there rather than at a nested path.
			name: "a compound setting resolved from a secret",
			setup: func(cfg pkgconfigmodel.Config) {
				cfg.Set("additional_endpoints", map[string][]string{
					"https://app.datadoghq.com": {"resolved-api-key"},
				}, pkgconfigmodel.SourceSecret)
			},
			want:       false,
			wantReason: "additional_endpoints",
		},
		{
			// The secrets layer sits below agent-runtime, RC and CLI, so a resolved value can
			// be shadowed in the merged view while still being present in the layer -- and
			// AllSettings, which is what gets written out, still merges it.
			name: "a shadowed value in the secrets layer",
			setup: func(cfg pkgconfigmodel.Config) {
				cfg.Set("api_key", "resolved-api-key", pkgconfigmodel.SourceSecret)
				cfg.Set("api_key", "override", pkgconfigmodel.SourceCLI)
			},
			want:       false,
			wantReason: "api_key",
		},
		{
			// Non-secret layers must not trip the walk, otherwise the pre-flight never runs.
			name: "values in other layers are not secrets",
			env:  map[string]string{"DD_PROXY_HTTPS": "https://proxy:3128"},
			setup: func(cfg pkgconfigmodel.Config) {
				cfg.Set("api_key", "plain-api-key", pkgconfigmodel.SourceFile)
				cfg.Set("hostname", "from-fleet-policies", pkgconfigmodel.SourceFleetPolicies)
				cfg.Set("tags", []string{"from:rc"}, pkgconfigmodel.SourceRC)
				cfg.Set("log_level", "debug", pkgconfigmodel.SourceCLI)
				cfg.Set("dd_url", "https://app.datadoghq.com", pkgconfigmodel.SourceAgentRuntime)
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			cfg := configmock.New(t)
			if tt.setup != nil {
				tt.setup(cfg)
			}
			got, reason := isEligible(cfg)
			assert.Equal(t, tt.want, got)
			if tt.want {
				assert.Empty(t, reason, "an eligible config must not come with a reason")
				return
			}
			assert.NotEmpty(t, reason, "an ineligible config must explain itself")
			if tt.wantReason != "" {
				assert.Contains(t, reason, tt.wantReason)
			}
		})
	}
}

// TestIsEligibleReasonDoesNotLeakSecrets pins that the reason we log names the setting and not its
// value. The whole point of the secrets gate is to keep resolved secrets out of anything durable,
// and the Agent log is durable.
func TestIsEligibleReasonDoesNotLeakSecrets(t *testing.T) {
	const secret = "s3cr3t-value-that-must-not-be-logged"

	for _, setting := range []string{secretBackendCommand, secretBackendType} {
		t.Run(setting, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.Set(setting, secret, pkgconfigmodel.SourceFile)

			eligible, reason := isEligible(cfg)
			assert.False(t, eligible)
			assert.NotContains(t, reason, secret)
		})
	}

	t.Run("resolved value", func(t *testing.T) {
		cfg := configmock.New(t)
		cfg.Set("api_key", secret, pkgconfigmodel.SourceSecret)

		eligible, reason := isEligible(cfg)
		assert.False(t, eligible)
		assert.NotContains(t, reason, secret)
	})
}

func TestFirstSecretSetting(t *testing.T) {
	t.Run("no secrets", func(t *testing.T) {
		cfg := configmock.New(t)

		found, setting := getFirstSecretSetting(cfg)
		assert.False(t, found)
		assert.Empty(t, setting)
	})

	t.Run("reports the setting it found", func(t *testing.T) {
		cfg := configmock.New(t)
		cfg.Set("hostname", "resolved-hostname", pkgconfigmodel.SourceSecret)

		found, setting := getFirstSecretSetting(cfg)
		assert.True(t, found)
		assert.Equal(t, "hostname", setting)
	})

	t.Run("is deterministic across several secrets", func(t *testing.T) {
		cfg := configmock.New(t)
		cfg.Set("hostname", "resolved-hostname", pkgconfigmodel.SourceSecret)
		cfg.Set("api_key", "resolved-api-key", pkgconfigmodel.SourceSecret)

		// AllKeysLowercased is sorted, so the lexicographically first of the two wins.
		for range 5 {
			found, setting := getFirstSecretSetting(cfg)
			assert.True(t, found)
			assert.Equal(t, "api_key", setting)
		}
	})
}
