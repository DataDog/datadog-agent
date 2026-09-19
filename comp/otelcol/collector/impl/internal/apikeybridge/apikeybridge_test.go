// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package apikeybridge

import (
	"testing"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/agentcomponents"
	datadogconfig "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configopaque"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	defaultforwarderimpl "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/impl"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

const testSite = "datadoghq.com"

// newExtensionConfig builds the private config component the way the upstream
// datadogextension does, with the bridge appended as a host option.
func newExtensionConfig(t *testing.T, extensionKey string, shared coreconfig.Component, logger log.Component) coreconfig.Component {
	t.Helper()
	ddCfg := &datadogconfig.Config{
		API: datadogconfig.APIConfig{
			Key:  configopaque.String(extensionKey),
			Site: testSite,
		},
	}
	return agentcomponents.NewConfigComponent(
		agentcomponents.WithAPIConfig(ddCfg),
		agentcomponents.WithForwarderConfig(),
		agentcomponents.WithPayloadsConfig(),
		agentcomponents.WithLoggingConfig(),
		Option(shared, logger),
	)
}

// newSharedConfig builds a stand-in for the Agent's live config component.
//
// The base value is written at SourceConfigPostInit (rank 6) rather than through
// NewMockWithOverrides, which uses SourceUnknown (rank 1): an ambient DD_API_KEY in the
// environment lands at SourceEnvVar (rank 4) and would otherwise mask it, making these
// tests depend on the developer's shell. Rank 6 sits above SourceEnvVar and below the
// ranks real rotations arrive at -- SourceSecret (7) and SourceLocalConfigProcess (8) --
// so those still win, as they do in production.
func newSharedConfig(t *testing.T, key string) coreconfig.Component {
	t.Helper()
	cfg := coreconfig.NewMock(t)
	cfg.Set("api_key", key, pkgconfigmodel.SourceConfigPostInit)
	cfg.Set("site", testSite, pkgconfigmodel.SourceConfigPostInit)
	require.Equal(t, key, cfg.GetString("api_key"), "test setup: api_key not effective")
	return cfg
}

// TestBridgePropagatesRotation is the core behaviour: a key change on the Agent's
// shared config reaches the extension's private config.
func TestBridgePropagatesRotation(t *testing.T) {
	shared := newSharedConfig(t, "key-old")
	priv := newExtensionConfig(t, "key-old", shared, logmock.New(t))

	require.Equal(t, "key-old", priv.GetString("api_key"))

	shared.Set("api_key", "key-new", pkgconfigmodel.SourceLocalConfigProcess)

	assert.Equal(t, "key-new", priv.GetString("api_key"))
}

// TestBridgeRotatesFromSecretSource covers the DDOT standalone path, where the writer is
// the otel-agent's own secrets backend at SourceSecret rather than configsync. The bridge
// subscribes to the config, not to a particular writer, so both must work.
func TestBridgeRotatesFromSecretSource(t *testing.T) {
	shared := newSharedConfig(t, "key-old")
	priv := newExtensionConfig(t, "key-old", shared, logmock.New(t))

	shared.Set("api_key", "key-from-secret", pkgconfigmodel.SourceSecret)

	assert.Equal(t, "key-from-secret", priv.GetString("api_key"))
}

// TestBridgeSkipsUserSuppliedKey verifies a user-declared datadog/<name> extension with
// its own explicit key is left alone, matching the converter's reuseExtension behaviour.
func TestBridgeSkipsUserSuppliedKey(t *testing.T) {
	shared := newSharedConfig(t, "agent-key")
	priv := newExtensionConfig(t, "user-supplied-key", shared, logmock.New(t))

	shared.Set("api_key", "agent-key-rotated", pkgconfigmodel.SourceLocalConfigProcess)

	assert.Equal(t, "user-supplied-key", priv.GetString("api_key"),
		"the bridge must not overwrite a key the user set explicitly")
}

// TestBridgeIgnoresOtherSettings verifies unrelated config changes are not mirrored.
func TestBridgeIgnoresOtherSettings(t *testing.T) {
	shared := newSharedConfig(t, "key-old")
	priv := newExtensionConfig(t, "key-old", shared, logmock.New(t))

	shared.Set("site", "datadoghq.eu", pkgconfigmodel.SourceLocalConfigProcess)

	assert.Equal(t, "key-old", priv.GetString("api_key"))
	assert.Equal(t, testSite, priv.GetString("site"), "site rotation is explicitly out of scope")
}

// TestBridgeUsesEffectiveValueNotCallbackValue guards the source-precedence subtlety: the
// notification reports the value for the source that changed, which a higher-ranked
// source may be masking. The extension must end up with the key the rest of the Agent is
// actually using, not the masked one.
func TestBridgeUsesEffectiveValueNotCallbackValue(t *testing.T) {
	shared := newSharedConfig(t, "key-old")
	priv := newExtensionConfig(t, "key-old", shared, logmock.New(t))

	// A high-ranked source pins the effective value.
	shared.Set("api_key", "key-from-cli", pkgconfigmodel.SourceCLI)
	require.Equal(t, "key-from-cli", shared.GetString("api_key"))

	// A lower-ranked writer now fires with a different value. It is masked, so the
	// extension must receive the CLI value, not "key-from-secret".
	shared.Set("api_key", "key-from-secret", pkgconfigmodel.SourceSecret)

	require.Equal(t, "key-from-cli", shared.GetString("api_key"), "precondition: CLI still wins")
	assert.Equal(t, "key-from-cli", priv.GetString("api_key"))
}

// TestBridgeNoAgentKey verifies the bridge arms nothing when the Agent has no key.
func TestBridgeNoAgentKey(t *testing.T) {
	shared := newSharedConfig(t, "")
	priv := newExtensionConfig(t, "some-key", shared, logmock.New(t))

	shared.Set("api_key", "key-new", pkgconfigmodel.SourceLocalConfigProcess)

	assert.Equal(t, "some-key", priv.GetString("api_key"))
}

// TestBridgeToleratesNilLogger verifies the logger is optional.
func TestBridgeToleratesNilLogger(t *testing.T) {
	shared := newSharedConfig(t, "key-old")
	priv := newExtensionConfig(t, "different-key", shared, nil)

	assert.NotPanics(t, func() {
		shared.Set("api_key", "key-new", pkgconfigmodel.SourceLocalConfigProcess)
	})
	assert.Equal(t, "different-key", priv.GetString("api_key"))
}

// TestBridgeRotatesForwarderResolver is the test that proves the reported bug is fixed.
// It arms the *production* rotation hook — the same resolver.OnUpdateConfig that
// NewOptionsWithResolvers registers inside the extension's forwarder — on the extension's
// private config, then rotates the Agent's key and asserts the resolver picked it up.
//
// Without the bridge the private config is never written to, the hook never fires, and
// the resolver keeps serving the stale key forever.
func TestBridgeRotatesForwarderResolver(t *testing.T) {
	logger := logmock.New(t)
	shared := newSharedConfig(t, "key-old")
	priv := newExtensionConfig(t, "key-old", shared, logger)

	const domain = "https://api." + testSite
	resolvers, err := resolver.NewSingleDomainResolvers(map[string][]utils.APIKeys{
		domain: {utils.NewAPIKeys("api_key", "key-old")},
	})
	require.NoError(t, err)

	// This is what agentcomponents.newForwarderComponent does inside the extension.
	defaultforwarderimpl.NewOptionsWithResolvers(priv, logger, resolvers)
	require.Equal(t, []string{"key-old"}, resolvers[domain].GetAPIKeys())

	shared.Set("api_key", "key-new", pkgconfigmodel.SourceLocalConfigProcess)

	assert.Equal(t, []string{"key-new"}, resolvers[domain].GetAPIKeys(),
		"the extension's forwarder did not rotate to the new API key")
}

// TestBridgeRotatesRepeatedly covers more than one rotation in the life of a process, which
// is what a secrets backend on a refresh interval produces. The tracking decision is taken
// once, on the first rotation: recomputing it would compare the extension's already-rotated
// key against the key captured at arm time, find them different, and silently stop following
// the Agent from the second rotation onwards.
func TestBridgeRotatesRepeatedly(t *testing.T) {
	logger := logmock.New(t)
	shared := newSharedConfig(t, "key-1")
	priv := newExtensionConfig(t, "key-1", shared, logger)

	const domain = "https://api." + testSite
	resolvers, err := resolver.NewSingleDomainResolvers(map[string][]utils.APIKeys{
		domain: {utils.NewAPIKeys("api_key", "key-1")},
	})
	require.NoError(t, err)
	defaultforwarderimpl.NewOptionsWithResolvers(priv, logger, resolvers)

	for _, key := range []string{"key-2", "key-3", "key-4"} {
		shared.Set("api_key", key, pkgconfigmodel.SourceLocalConfigProcess)

		assert.Equal(t, key, priv.GetString("api_key"),
			"rotation to %q did not reach the extension's config", key)
		assert.Equal(t, []string{key}, resolvers[domain].GetAPIKeys(),
			"rotation to %q did not reach the extension's forwarder", key)
	}
}
