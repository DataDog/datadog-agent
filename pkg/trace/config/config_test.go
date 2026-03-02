// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
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

func TestNew(t *testing.T) {
	cfg := New()

	// Test default values
	assert.True(t, cfg.Enabled)
	assert.Equal(t, "none", cfg.DefaultEnv)
	assert.Equal(t, "datadoghq.com", cfg.Site)
	assert.Equal(t, OrchestratorUnknown, cfg.FargateOrchestrator)
	assert.Equal(t, 5000, cfg.MaxCatalogEntries)

	// Test receiver defaults
	assert.True(t, cfg.ReceiverEnabled)
	assert.Equal(t, "localhost", cfg.ReceiverHost)
	assert.Equal(t, 8126, cfg.ReceiverPort)
	assert.Equal(t, int64(25*1024*1024), cfg.MaxRequestBytes)

	// Test stats defaults
	assert.True(t, cfg.StatsdEnabled)
	assert.Equal(t, "localhost", cfg.StatsdHost)
	assert.Equal(t, 8125, cfg.StatsdPort)

	// Test sampler defaults
	assert.Equal(t, 1.0, cfg.ExtraSampleRate)
	assert.Equal(t, 10.0, cfg.TargetTPS)
	assert.Equal(t, 10.0, cfg.ErrorTPS)
	assert.Equal(t, 200.0, cfg.MaxEPS)

	// Test rare sampler defaults
	assert.False(t, cfg.RareSamplerEnabled)
	assert.Equal(t, 5, cfg.RareSamplerTPS)
	assert.Equal(t, 200, cfg.RareSamplerCardinality)

	// Test EVP proxy defaults
	assert.True(t, cfg.EVPProxy.Enabled)
	assert.Equal(t, int64(10*1024*1024), cfg.EVPProxy.MaxPayloadSize)

	// Test peer tags aggregation defaults
	assert.True(t, cfg.PeerTagsAggregation)
	assert.True(t, cfg.ComputeStatsBySpanKind)
}

func TestAPIKey(t *testing.T) {
	t.Run("empty endpoints", func(t *testing.T) {
		cfg := New()
		cfg.Endpoints = []*Endpoint{}
		assert.Equal(t, "", cfg.APIKey())
	})

	t.Run("with endpoint", func(t *testing.T) {
		cfg := New()
		cfg.Endpoints = []*Endpoint{{APIKey: "test-api-key"}}
		assert.Equal(t, "test-api-key", cfg.APIKey())
	})
}

func TestUpdateAPIKey(t *testing.T) {
	t.Run("empty endpoints", func(t *testing.T) {
		cfg := New()
		cfg.Endpoints = []*Endpoint{}
		cfg.UpdateAPIKey("new-key")
		// Should not panic, just do nothing
		assert.Empty(t, cfg.Endpoints)
	})

	t.Run("with endpoint", func(t *testing.T) {
		cfg := New()
		cfg.Endpoints = []*Endpoint{{APIKey: "old-key"}}
		cfg.UpdateAPIKey("new-key")
		assert.Equal(t, "new-key", cfg.Endpoints[0].APIKey)
	})
}

func TestHasFeature(t *testing.T) {
	cfg := New()

	// Feature not present
	assert.False(t, cfg.HasFeature("nonexistent"))

	// Add feature
	cfg.Features["test_feature"] = struct{}{}
	assert.True(t, cfg.HasFeature("test_feature"))
}

func TestAllFeatures(t *testing.T) {
	cfg := New()

	// Empty features
	assert.Empty(t, cfg.AllFeatures())

	// Add features
	cfg.Features["feature1"] = struct{}{}
	cfg.Features["feature2"] = struct{}{}

	features := cfg.AllFeatures()
	assert.Len(t, features, 2)
	assert.Contains(t, features, "feature1")
	assert.Contains(t, features, "feature2")
}

func TestNewHTTPTransport(t *testing.T) {
	t.Run("default transport", func(t *testing.T) {
		cfg := New()
		transport := cfg.NewHTTPTransport()
		assert.NotNil(t, transport)
		assert.NotNil(t, transport.TLSClientConfig)
		assert.False(t, transport.TLSClientConfig.InsecureSkipVerify)
	})

	t.Run("skip SSL validation", func(t *testing.T) {
		cfg := New()
		cfg.SkipSSLValidation = true
		transport := cfg.NewHTTPTransport()
		assert.NotNil(t, transport)
		assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
	})

	t.Run("custom transport factory", func(t *testing.T) {
		cfg := New()
		customCalled := false
		cfg.HTTPTransportFunc = func() *http.Transport {
			customCalled = true
			return &http.Transport{}
		}
		transport := cfg.NewHTTPTransport()
		assert.NotNil(t, transport)
		assert.True(t, customCalled)
	})
}

func TestNewHTTPClient(t *testing.T) {
	cfg := New()
	client := cfg.NewHTTPClient()
	assert.NotNil(t, client)
}

func TestNoopContainerTagsFunc(t *testing.T) {
	tags, err := noopContainerTagsFunc("container-id")
	assert.Nil(t, tags)
	assert.Error(t, err)
	assert.Equal(t, ErrContainerTagsFuncNotDefined, err)
}

func TestNoopContainerIDFromOriginInfoFunc(t *testing.T) {
	id, err := NoopContainerIDFromOriginInfoFunc(origindetection.OriginInfo{})
	assert.Equal(t, "", id)
	assert.Error(t, err)
	assert.Equal(t, ErrContainerIDFromOriginInfoFuncNotDefined, err)
}

func TestObfuscationConfigExport(t *testing.T) {
	cfg := New()
	cfg.Features["table_names"] = struct{}{}
	cfg.Features["keep_sql_alias"] = struct{}{}

	o := &ObfuscationConfig{
		RemoveStackTraces: true,
	}

	exported := o.Export(cfg)
	assert.True(t, exported.SQL.TableNames)
	assert.True(t, exported.SQL.KeepSQLAlias)
	assert.False(t, exported.SQL.ReplaceDigits)
}

func TestInvalidSQLObfuscationMode(t *testing.T) {
	cfg := New()
	cfg.SQLObfuscationMode = "invalid_mode"
	mode := obfuscationMode(cfg, false)
	assert.Equal(t, obfuscate.ObfuscationMode(""), mode)
}
