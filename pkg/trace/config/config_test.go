// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	"github.com/DataDog/datadog-agent/pkg/trace/semantics"
)

const (
	WebsiteStack = "WEBSITE_STACK"
	AppLogsTrace = "WEBSITE_APPSERVICEAPPLOGS_TRACE_ENABLED"
)

func TestIsQueryAttributeAllowed(t *testing.T) {
	tests := []struct {
		name      string
		config    *AgentConfig
		attribute string
		want      bool
	}{
		{name: "nil config", attribute: QueryAttributeSQLQuery, want: true},
		{name: "nil allowlist", config: &AgentConfig{}, attribute: QueryAttributeSQLQuery, want: true},
		{name: "empty allowlist", config: &AgentConfig{QueryAttributeAllowlist: []string{}}, attribute: QueryAttributeSQLQuery, want: true},
		{name: "none", config: &AgentConfig{QueryAttributeAllowlist: []string{QueryAttributeNone}}, attribute: QueryAttributeSQLQuery, want: false},
		{name: "allowed subset member", config: &AgentConfig{QueryAttributeAllowlist: []string{QueryAttributeDBQueryText}}, attribute: QueryAttributeDBQueryText, want: true},
		{name: "excluded subset member", config: &AgentConfig{QueryAttributeAllowlist: []string{QueryAttributeDBQueryText}}, attribute: QueryAttributeSQLQuery, want: false},
		{name: "unknown value", config: &AgentConfig{QueryAttributeAllowlist: []string{"unknown"}}, attribute: QueryAttributeSQLQuery, want: false},
		{name: "none wins when mixed", config: &AgentConfig{QueryAttributeAllowlist: []string{QueryAttributeNone, QueryAttributeSQLQuery}}, attribute: QueryAttributeSQLQuery, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.config.IsQueryAttributeAllowed(tt.attribute))
		})
	}
}

func TestValidateQueryAttributeAllowlist(t *testing.T) {
	for _, tt := range []struct {
		name      string
		allowlist []string
		wantError string
	}{
		{name: "nil"},
		{name: "empty", allowlist: []string{}},
		{name: "none", allowlist: []string{QueryAttributeNone}},
		{name: "subset", allowlist: []string{QueryAttributeSQLQuery, QueryAttributeDBQueryText}},
		{name: "unknown", allowlist: []string{"unknown"}, wantError: "unsupported database query attribute"},
		{name: "none mixed", allowlist: []string{QueryAttributeNone, QueryAttributeSQLQuery}, wantError: "cannot be combined"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateQueryAttributeAllowlist(tt.allowlist)
			if tt.wantError == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantError)
			}
		})
	}
}

func TestInAzureAppServices(t *testing.T) {
	os.Setenv(WebsiteStack, " ")
	isLinuxAzure := inAzureAppServices()
	os.Unsetenv(WebsiteStack)

	os.Setenv(AppLogsTrace, " ")
	isWindowsAzure := inAzureAppServices()
	os.Unsetenv(AppLogsTrace)

	isNotAzure := inAzureAppServices()

	assert.True(t, isLinuxAzure)
	assert.True(t, isWindowsAzure)
	assert.False(t, isNotAzure)
}

func TestSpanDerivedPrimaryTagKeys(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		cfg := New()
		assert.Empty(t, cfg.SpanDerivedPrimaryTagKeys)
		assert.Empty(t, cfg.ConfiguredSpanDerivedPrimaryTagKeys())
	})

	t.Run("configured", func(t *testing.T) {
		cfg := New()
		cfg.SpanDerivedPrimaryTagKeys = []string{"datacenter", "customer_tier", "availability_zone"}
		assert.Equal(t, []string{"availability_zone", "customer_tier", "datacenter"}, cfg.ConfiguredSpanDerivedPrimaryTagKeys())
	})

	t.Run("dedup", func(t *testing.T) {
		cfg := New()
		cfg.SpanDerivedPrimaryTagKeys = []string{"datacenter", "customer_tier", "datacenter"}
		assert.Equal(t, []string{"customer_tier", "datacenter"}, cfg.ConfiguredSpanDerivedPrimaryTagKeys())
	})
}

func TestMRFFailoverAPM(t *testing.T) {
	t.Run("undefined", func(t *testing.T) {
		cfg := New()
		assert.False(t, cfg.MRFFailoverAPM())
	})
	t.Run("default-true", func(t *testing.T) {
		cfg := New()
		cfg.MRFFailoverAPMDefault = true
		assert.True(t, cfg.MRFFailoverAPM())
	})
	t.Run("default-false", func(t *testing.T) {
		cfg := New()
		cfg.MRFFailoverAPMDefault = false
		assert.False(t, cfg.MRFFailoverAPM())
	})
	t.Run("rc-true", func(t *testing.T) {
		cfg := New()
		cfg.MRFFailoverAPMDefault = false
		val := true
		cfg.MRFFailoverAPMRC = &val
		assert.True(t, cfg.MRFFailoverAPM())
	})
	t.Run("rc-false", func(t *testing.T) {
		cfg := New()
		cfg.MRFFailoverAPMDefault = true
		val := false
		cfg.MRFFailoverAPMRC = &val
		assert.False(t, cfg.MRFFailoverAPM())
	})
	// Test that RC overrides can be removed (set to nil)
	t.Run("rc-unset", func(t *testing.T) {
		cfg := New()
		cfg.MRFFailoverAPMDefault = true
		val := false
		cfg.MRFFailoverAPMRC = &val
		assert.False(t, cfg.MRFFailoverAPM())
		cfg.MRFFailoverAPMRC = nil
		assert.True(t, cfg.MRFFailoverAPM())
	})
}

func TestSQLObfuscationMode(t *testing.T) {
	t.Run("normalize_only", func(t *testing.T) {
		cfg := New()
		cfg.SQLObfuscationMode = "normalize_only"
		assert.Equal(t, obfuscate.NormalizeOnly, obfuscationMode(cfg, false))
	})
	t.Run("obfuscate_only", func(t *testing.T) {
		cfg := New()
		cfg.SQLObfuscationMode = "obfuscate_only"
		assert.Equal(t, obfuscate.ObfuscateOnly, obfuscationMode(cfg, false))
	})
	t.Run("obfuscate_and_normalize", func(t *testing.T) {
		cfg := New()
		cfg.SQLObfuscationMode = "obfuscate_and_normalize"
		assert.Equal(t, obfuscate.ObfuscateAndNormalize, obfuscationMode(cfg, false))
	})
	t.Run("empty", func(t *testing.T) {
		cfg := New()
		assert.Equal(t, obfuscate.ObfuscationMode(""), obfuscationMode(cfg, false))
	})
	t.Run("sqlexer", func(t *testing.T) {
		cfg := New()
		assert.Equal(t, obfuscate.ObfuscateOnly, obfuscationMode(cfg, true))
	})
}

func TestEffectiveSQLObfuscationMode(t *testing.T) {
	t.Run("sqllexer_enabled_no_explicit_mode", func(t *testing.T) {
		cfg := New()
		cfg.Features = map[string]struct{}{"sqllexer": {}}
		// SQLObfuscationMode is empty; effective mode must be obfuscate_only
		assert.Equal(t, obfuscate.ObfuscateOnly, cfg.EffectiveSQLObfuscationMode())
	})
	t.Run("explicit_mode_takes_precedence", func(t *testing.T) {
		cfg := New()
		cfg.Features = map[string]struct{}{"sqllexer": {}}
		cfg.SQLObfuscationMode = string(obfuscate.ObfuscateAndNormalize)
		assert.Equal(t, obfuscate.ObfuscateAndNormalize, cfg.EffectiveSQLObfuscationMode())
	})
	t.Run("no_sqllexer_no_explicit_mode", func(t *testing.T) {
		cfg := New()
		assert.Equal(t, obfuscate.ObfuscationMode(""), cfg.EffectiveSQLObfuscationMode())
	})
}

func TestInECSManagedInstancesSidecar(t *testing.T) {
	t.Setenv("DD_ECS_DEPLOYMENT_MODE", "sidecar")
	t.Setenv("AWS_EXECUTION_ENV", "AWS_ECS_MANAGED_INSTANCES")
	isSidecar := inECSManagedInstancesSidecar()

	assert.True(t, isSidecar)
}

func TestDefaultAPMMode(t *testing.T) {
	t.Run("default-empty", func(t *testing.T) {
		cfg := New()
		assert.Empty(t, cfg.APMMode)
	})
}

func TestEnableOPMFetchDefault(t *testing.T) {
	cfg := New()
	assert.False(t, cfg.EnableOPMFetch, "EnableOPMFetch must default to false so library users of pkg/trace are unaffected")
	assert.Empty(t, cfg.OPMValidateURL, "OPMValidateURL must default to empty when EnableOPMFetch is false")
}

func TestConfiguredPeerTagsUsesLiveRegistry(t *testing.T) {
	// Custom registry: ConceptPeerService maps to "x.custom.peer" instead of "peer.service".
	customJSON := `{"version":"test","metadata":{"content_hash":"hash-a"},"concepts":{"peer.service":{"canonical":"peer.service","fallbacks":[{"name":"x.custom.peer","provider":"datadog","type":"string"}]}}}`
	custom, err := semantics.NewRegistryFromJSON([]byte(customJSON))
	require.NoError(t, err)
	original, err := semantics.NewEmbeddedRegistry()
	require.NoError(t, err)
	t.Cleanup(func() { semantics.UpdateRegistry(original) })

	semantics.UpdateRegistry(custom)

	cfg := New()
	tags := cfg.ConfiguredPeerTags()
	assert.Contains(t, tags, "x.custom.peer")
	assert.NotContains(t, tags, "peer.service")
}
