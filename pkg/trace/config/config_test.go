// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/obfuscate"
)

const (
	WebsiteStack = "WEBSITE_STACK"
	AppLogsTrace = "WEBSITE_APPSERVICEAPPLOGS_TRACE_ENABLED"
)

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

func TestPeerTagsAggregation(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		cfg := New()
		cfg.PeerTagsAggregation = false
		assert.False(t, cfg.PeerTagsAggregation)
		assert.Empty(t, cfg.PeerTags)
		assert.Empty(t, cfg.ConfiguredPeerTags())
	})

	t.Run("default-enabled", func(t *testing.T) {
		cfg := New()
		assert.Empty(t, cfg.PeerTags)
		assert.Equal(t, basePeerTags, cfg.ConfiguredPeerTags())
	})
	t.Run("disabled-user-tags", func(t *testing.T) {
		cfg := New()
		cfg.PeerTagsAggregation = false
		cfg.PeerTags = []string{"user_peer_tag"}
		assert.False(t, cfg.PeerTagsAggregation)
		assert.Empty(t, cfg.ConfiguredPeerTags())
	})
	t.Run("enabled-user-tags", func(t *testing.T) {
		cfg := New()
		cfg.PeerTags = []string{"user_peer_tag"}
		assert.Equal(t, append(basePeerTags, "user_peer_tag"), cfg.ConfiguredPeerTags())
	})
	t.Run("dedup", func(t *testing.T) {
		cfg := New()
		cfg.PeerTags = basePeerTags[:2]
		assert.Equal(t, basePeerTags, cfg.ConfiguredPeerTags())
	})
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
