// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package remoteconfighandler

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state/products/apmsampling"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/semantics"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

// restoreEmbeddedRegistry installs a t.Cleanup that resets the global registry
// to the embedded mappings.json. Required because the registry is process-wide
// and tests must not leave it mutated for other tests.
func restoreEmbeddedRegistry(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		r, err := semantics.NewEmbeddedRegistry()
		require.NoError(t, err)
		semantics.UpdateRegistry(r)
	})
}

// nolint: revive
func applyEmpty(_ string, _ state.ApplyStatus) {}

func TestStart(t *testing.T) {
	ctrl := gomock.NewController(t)
	remoteClient := config.NewMockRemoteClient(ctrl)
	agentConfig := config.AgentConfig{
		RemoteConfigClient:             remoteClient,
		DebugServerPort:                1,
		RemoteConfigAPMSamplingEnabled: true,
		RemoteConfigAgentConfigEnabled: true,
	}
	prioritySampler := NewMockprioritySampler(ctrl)
	errorsSampler := NewMockerrorsSampler(ctrl)
	rareSampler := NewMockrareSampler(ctrl)
	pkglog.SetupLogger(pkglog.Default(), "debug")

	h := New(&agentConfig, prioritySampler, rareSampler, errorsSampler)

	remoteClient.EXPECT().Subscribe(state.ProductAPMSampling, gomock.Any()).Times(1)
	remoteClient.EXPECT().Subscribe(state.ProductAgentConfig, gomock.Any()).Times(1)
	remoteClient.EXPECT().Start().Times(1)

	h.Start()
}

func TestStart_AllFlagsEnabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	remoteClient := config.NewMockRemoteClient(ctrl)
	agentConfig := config.AgentConfig{
		RemoteConfigClient:              remoteClient,
		DebugServerPort:                 1,
		RemoteConfigAPMSamplingEnabled:  true,
		RemoteConfigAgentConfigEnabled:  true,
		RemoteConfigAPMSemanticsEnabled: true,
	}
	prioritySampler := NewMockprioritySampler(ctrl)
	errorsSampler := NewMockerrorsSampler(ctrl)
	rareSampler := NewMockrareSampler(ctrl)
	pkglog.SetupLogger(pkglog.Default(), "debug")

	h := New(&agentConfig, prioritySampler, rareSampler, errorsSampler)

	remoteClient.EXPECT().Subscribe(state.ProductAPMSampling, gomock.Any()).Times(1)
	remoteClient.EXPECT().Subscribe(state.ProductAgentConfig, gomock.Any()).Times(1)
	remoteClient.EXPECT().Subscribe(state.ProductAPMSemanticCoreDD, gomock.Any()).Times(1)
	remoteClient.EXPECT().Start().Times(1)

	h.Start()
}

func TestStart_OnlySemanticsEnabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	remoteClient := config.NewMockRemoteClient(ctrl)
	agentConfig := config.AgentConfig{
		RemoteConfigClient:              remoteClient,
		DebugServerPort:                 1,
		RemoteConfigAPMSemanticsEnabled: true,
	}
	prioritySampler := NewMockprioritySampler(ctrl)
	errorsSampler := NewMockerrorsSampler(ctrl)
	rareSampler := NewMockrareSampler(ctrl)
	pkglog.SetupLogger(pkglog.Default(), "debug")

	h := New(&agentConfig, prioritySampler, rareSampler, errorsSampler)

	// Only APM_SEMANTIC_CORE_DD — AGENT_CONFIG and APM_SAMPLING must NOT be pulled in.
	remoteClient.EXPECT().Subscribe(state.ProductAPMSemanticCoreDD, gomock.Any()).Times(1)
	remoteClient.EXPECT().Start().Times(1)

	h.Start()
}

func TestStart_OnlyAgentConfigEnabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	remoteClient := config.NewMockRemoteClient(ctrl)
	agentConfig := config.AgentConfig{
		RemoteConfigClient:             remoteClient,
		DebugServerPort:                1,
		RemoteConfigAgentConfigEnabled: true,
	}
	prioritySampler := NewMockprioritySampler(ctrl)
	errorsSampler := NewMockerrorsSampler(ctrl)
	rareSampler := NewMockrareSampler(ctrl)
	pkglog.SetupLogger(pkglog.Default(), "debug")

	h := New(&agentConfig, prioritySampler, rareSampler, errorsSampler)

	remoteClient.EXPECT().Subscribe(state.ProductAgentConfig, gomock.Any()).Times(1)
	remoteClient.EXPECT().Start().Times(1)

	h.Start()
}

func TestStartNoRemoteClient(t *testing.T) {
	var h *RemoteConfigHandler
	assert.NotPanics(t, h.Start)
}

// TestNew_NoDebugServerWithSemanticsOnly verifies that the handler is built
// successfully when the debug server is disabled but apm_semantics RC is
// requested — semantics RC does not depend on the debug server.
func TestNew_NoDebugServerWithSemanticsOnly(t *testing.T) {
	ctrl := gomock.NewController(t)
	remoteClient := config.NewMockRemoteClient(ctrl)
	agentConfig := config.AgentConfig{
		RemoteConfigClient:              remoteClient,
		DebugServerPort:                 0,
		RemoteConfigAPMSemanticsEnabled: true,
	}
	pkglog.SetupLogger(pkglog.Default(), "debug")

	h := New(&agentConfig, NewMockprioritySampler(ctrl), NewMockrareSampler(ctrl), NewMockerrorsSampler(ctrl))
	require.NotNil(t, h, "handler must be constructed when a debug-server-independent product is enabled")

	remoteClient.EXPECT().Subscribe(state.ProductAPMSemanticCoreDD, gomock.Any()).Times(1)
	remoteClient.EXPECT().Start().Times(1)
	h.Start()
}

// TestNew_NoDebugServerNoOtherProducts verifies that the handler returns nil
// when the debug server is disabled and only AGENT_CONFIG could have been
// requested — there is nothing useful for the handler to do.
func TestNew_NoDebugServerNoOtherProducts(t *testing.T) {
	ctrl := gomock.NewController(t)
	remoteClient := config.NewMockRemoteClient(ctrl)
	agentConfig := config.AgentConfig{
		RemoteConfigClient:             remoteClient,
		DebugServerPort:                0,
		RemoteConfigAgentConfigEnabled: true,
	}
	pkglog.SetupLogger(pkglog.Default(), "debug")

	h := New(&agentConfig, NewMockprioritySampler(ctrl), NewMockrareSampler(ctrl), NewMockerrorsSampler(ctrl))
	assert.Nil(t, h, "handler must be nil: debug server is disabled and AGENT_CONFIG is the only requested product")
}

// TestStart_AgentConfigSkippedWithoutDebugServer verifies that with a mix of
// APM_SAMPLING + AGENT_CONFIG requested but debug server disabled, the handler
// is still built and subscribes to APM_SAMPLING only; the AGENT_CONFIG
// subscription is skipped with a warning.
func TestStart_AgentConfigSkippedWithoutDebugServer(t *testing.T) {
	ctrl := gomock.NewController(t)
	remoteClient := config.NewMockRemoteClient(ctrl)
	agentConfig := config.AgentConfig{
		RemoteConfigClient:             remoteClient,
		DebugServerPort:                0,
		RemoteConfigAPMSamplingEnabled: true,
		RemoteConfigAgentConfigEnabled: true,
	}
	pkglog.SetupLogger(pkglog.Default(), "debug")

	h := New(&agentConfig, NewMockprioritySampler(ctrl), NewMockrareSampler(ctrl), NewMockerrorsSampler(ctrl))
	require.NotNil(t, h)

	// Only APM_SAMPLING is subscribed; AGENT_CONFIG is intentionally skipped.
	remoteClient.EXPECT().Subscribe(state.ProductAPMSampling, gomock.Any()).Times(1)
	remoteClient.EXPECT().Start().Times(1)
	h.Start()
}

func TestPrioritySampler(t *testing.T) {
	ctrl := gomock.NewController(t)
	remoteClient := config.NewMockRemoteClient(ctrl)
	prioritySampler := NewMockprioritySampler(ctrl)
	errorsSampler := NewMockerrorsSampler(ctrl)
	rareSampler := NewMockrareSampler(ctrl)
	pkglog.SetupLogger(pkglog.Default(), "debug")

	agentConfig := config.AgentConfig{RemoteConfigClient: remoteClient, TargetTPS: 41, ErrorTPS: 41, RareSamplerEnabled: true, DebugServerPort: 1}
	h := New(&agentConfig, prioritySampler, rareSampler, errorsSampler)

	payload := apmsampling.SamplerConfig{
		AllEnvs: apmsampling.SamplerEnvConfig{
			PrioritySamplerTargetTPS: pointer.Ptr(42.0),
		},
	}

	raw, _ := json.Marshal(payload)
	config := state.RawConfig{
		Config: raw,
	}

	prioritySampler.EXPECT().UpdateTargetTPS(float64(42)).Times(1)
	errorsSampler.EXPECT().UpdateTargetTPS(float64(41)).Times(1)
	rareSampler.EXPECT().SetEnabled(true).Times(1)

	h.onUpdate(map[string]state.RawConfig{"datadog/2/APM_SAMPLING/samplerconfig/config": config}, applyEmpty)
}

func TestErrorsSampler(t *testing.T) {
	ctrl := gomock.NewController(t)
	remoteClient := config.NewMockRemoteClient(ctrl)
	prioritySampler := NewMockprioritySampler(ctrl)
	errorsSampler := NewMockerrorsSampler(ctrl)
	rareSampler := NewMockrareSampler(ctrl)
	pkglog.SetupLogger(pkglog.Default(), "debug")

	agentConfig := config.AgentConfig{RemoteConfigClient: remoteClient, TargetTPS: 41, ErrorTPS: 41, RareSamplerEnabled: true, DebugServerPort: 1}
	h := New(&agentConfig, prioritySampler, rareSampler, errorsSampler)

	payload := apmsampling.SamplerConfig{
		AllEnvs: apmsampling.SamplerEnvConfig{
			ErrorsSamplerTargetTPS: pointer.Ptr(42.0),
		},
	}

	raw, _ := json.Marshal(payload)
	config := state.RawConfig{
		Config: raw,
	}

	prioritySampler.EXPECT().UpdateTargetTPS(float64(41)).Times(1)
	errorsSampler.EXPECT().UpdateTargetTPS(float64(42)).Times(1)
	rareSampler.EXPECT().SetEnabled(true).Times(1)

	h.onUpdate(map[string]state.RawConfig{"datadog/2/APM_SAMPLING/samplerconfig/config": config}, applyEmpty)
}

func TestRareSampler(t *testing.T) {
	ctrl := gomock.NewController(t)
	remoteClient := config.NewMockRemoteClient(ctrl)
	prioritySampler := NewMockprioritySampler(ctrl)
	errorsSampler := NewMockerrorsSampler(ctrl)
	rareSampler := NewMockrareSampler(ctrl)
	pkglog.SetupLogger(pkglog.Default(), "debug")

	agentConfig := config.AgentConfig{RemoteConfigClient: remoteClient, TargetTPS: 41, ErrorTPS: 41, RareSamplerEnabled: true, DebugServerPort: 1}
	h := New(&agentConfig, prioritySampler, rareSampler, errorsSampler)

	payload := apmsampling.SamplerConfig{
		AllEnvs: apmsampling.SamplerEnvConfig{
			RareSamplerEnabled: pointer.Ptr(false),
		},
	}

	raw, _ := json.Marshal(payload)
	config := state.RawConfig{
		Config: raw,
	}

	prioritySampler.EXPECT().UpdateTargetTPS(float64(41)).Times(1)
	errorsSampler.EXPECT().UpdateTargetTPS(float64(41)).Times(1)
	rareSampler.EXPECT().SetEnabled(false).Times(1)

	h.onUpdate(map[string]state.RawConfig{"datadog/2/APM_SAMPLING/samplerconfig/config": config}, applyEmpty)
}

func TestEnvPrecedence(t *testing.T) {
	ctrl := gomock.NewController(t)
	remoteClient := config.NewMockRemoteClient(ctrl)
	prioritySampler := NewMockprioritySampler(ctrl)
	errorsSampler := NewMockerrorsSampler(ctrl)
	rareSampler := NewMockrareSampler(ctrl)
	pkglog.SetupLogger(pkglog.Default(), "debug")

	agentConfig := config.AgentConfig{RemoteConfigClient: remoteClient, TargetTPS: 41, ErrorTPS: 41, RareSamplerEnabled: true, DefaultEnv: "agent-env", DebugServerPort: 1}
	h := New(&agentConfig, prioritySampler, rareSampler, errorsSampler)

	payload := apmsampling.SamplerConfig{
		AllEnvs: apmsampling.SamplerEnvConfig{
			PrioritySamplerTargetTPS: pointer.Ptr(42.0),
			ErrorsSamplerTargetTPS:   pointer.Ptr(42.0),
			RareSamplerEnabled:       pointer.Ptr(true),
		},
		ByEnv: []apmsampling.EnvAndConfig{{
			Env: "agent-env",
			Config: apmsampling.SamplerEnvConfig{
				PrioritySamplerTargetTPS: pointer.Ptr(43.0),
				ErrorsSamplerTargetTPS:   pointer.Ptr(43.0),
				RareSamplerEnabled:       pointer.Ptr(false),
			},
		}},
	}

	raw, _ := json.Marshal(payload)
	config := state.RawConfig{
		Config: raw,
	}

	prioritySampler.EXPECT().UpdateTargetTPS(float64(43)).Times(1)
	errorsSampler.EXPECT().UpdateTargetTPS(float64(43)).Times(1)
	rareSampler.EXPECT().SetEnabled(false).Times(1)

	h.onUpdate(map[string]state.RawConfig{"datadog/2/APM_SAMPLING/samplerconfig/config": config}, applyEmpty)
}

func TestLogLevel(t *testing.T) {
	ctrl := gomock.NewController(t)
	remoteClient := config.NewMockRemoteClient(ctrl)
	prioritySampler := NewMockprioritySampler(ctrl)
	errorsSampler := NewMockerrorsSampler(ctrl)
	rareSampler := NewMockrareSampler(ctrl)

	pkglog.SetupLogger(pkglog.Default(), "debug")
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer fakeToken", r.Header.Get("Authorization"))
		w.WriteHeader(200)
	}))
	defer srv.Close()
	port, _ := strconv.Atoi(strings.Split(srv.URL, ":")[2])

	agentConfig := config.AgentConfig{
		RemoteConfigClient: remoteClient,
		DefaultEnv:         "agent-env",
		DebugServerPort:    port,
		AuthToken:          "fakeToken",
		IPCTLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	h := New(&agentConfig, prioritySampler, rareSampler, errorsSampler)

	layer := state.RawConfig{Config: []byte(`{"name": "layer1", "config": {"log_level": "debug"}}`)}
	configOrder := state.RawConfig{Config: []byte(`{"internal_order": ["layer1", "layer2"]}`)}

	remoteClient.EXPECT().UpdateApplyStatus(
		"datadog/2/AGENT_CONFIG/layer1/configname",
		state.ApplyStatus{State: state.ApplyStateAcknowledged},
	)
	remoteClient.EXPECT().UpdateApplyStatus(
		"datadog/2/AGENT_CONFIG/configuration_order/configname",
		state.ApplyStatus{State: state.ApplyStateAcknowledged},
	)

	h.onAgentConfigUpdate(map[string]state.RawConfig{
		"datadog/2/AGENT_CONFIG/layer1/configname":              layer,
		"datadog/2/AGENT_CONFIG/configuration_order/configname": configOrder,
	}, remoteClient.UpdateApplyStatus)
}

func TestStartWithMRF(t *testing.T) {
	ctrl := gomock.NewController(t)
	remoteClient := config.NewMockRemoteClient(ctrl)
	mrfClient := config.NewMockRemoteClient(ctrl)
	agentConfig := config.AgentConfig{
		RemoteConfigClient:             remoteClient,
		MRFRemoteConfigClient:          mrfClient,
		DebugServerPort:                1,
		RemoteConfigAPMSamplingEnabled: true,
		RemoteConfigAgentConfigEnabled: true,
	}
	prioritySampler := NewMockprioritySampler(ctrl)
	errorsSampler := NewMockerrorsSampler(ctrl)
	rareSampler := NewMockrareSampler(ctrl)
	pkglog.SetupLogger(pkglog.Default(), "debug")

	h := New(&agentConfig, prioritySampler, rareSampler, errorsSampler)

	remoteClient.EXPECT().Subscribe(state.ProductAPMSampling, gomock.Any()).Times(1)
	remoteClient.EXPECT().Subscribe(state.ProductAgentConfig, gomock.Any()).Times(1)
	remoteClient.EXPECT().Start().Times(1)
	mrfClient.EXPECT().Subscribe(state.ProductAgentFailover, gomock.Any()).Times(1)
	mrfClient.EXPECT().Start().Times(1)

	h.Start()
}

func TestMRFUpdateCallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	remoteClient := config.NewMockRemoteClient(ctrl)
	mrfClient := config.NewMockRemoteClient(ctrl)
	prioritySampler := NewMockprioritySampler(ctrl)
	errorsSampler := NewMockerrorsSampler(ctrl)
	rareSampler := NewMockrareSampler(ctrl)
	pkglog.SetupLogger(pkglog.Default(), "debug")

	agentConfig := config.AgentConfig{
		RemoteConfigClient:    remoteClient,
		MRFRemoteConfigClient: mrfClient,
		DebugServerPort:       1,
	}
	h := New(&agentConfig, prioritySampler, rareSampler, errorsSampler)

	// Disabled by default
	assert.False(t, h.agentConfig.MRFFailoverAPM())

	// Test empty updates
	h.mrfUpdateCallback(map[string]state.RawConfig{}, applyEmpty)
	assert.False(t, h.agentConfig.MRFFailoverAPM())

	// Test enabling MRF
	mrfConfig := map[string]interface{}{
		"failover_apm": true,
	}
	raw, _ := json.Marshal(mrfConfig)
	config := state.RawConfig{
		Config: raw,
	}
	h.mrfUpdateCallback(map[string]state.RawConfig{"datadog/2/AGENT_FAILOVER/config": config}, applyEmpty)
	assert.True(t, h.agentConfig.MRFFailoverAPM())

	// Test disabling MRF
	mrfConfig = map[string]interface{}{
		"failover_apm": false,
	}
	raw, _ = json.Marshal(mrfConfig)
	config = state.RawConfig{
		Config: raw,
	}
	h.mrfUpdateCallback(map[string]state.RawConfig{"datadog/2/AGENT_FAILOVER/config": config}, applyEmpty)
	assert.False(t, h.agentConfig.MRFFailoverAPM())

	// Test empty updates
	h.mrfUpdateCallback(map[string]state.RawConfig{}, applyEmpty)
	assert.False(t, h.agentConfig.MRFFailoverAPM())

	// Test invalid config
	invalidConfig := state.RawConfig{
		Config: []byte(`invalid json`),
	}
	h.mrfUpdateCallback(map[string]state.RawConfig{"datadog/2/AGENT_FAILOVER/config": invalidConfig}, applyEmpty)
	assert.False(t, h.agentConfig.MRFFailoverAPM())
}

func TestMRFUpdateCallbackWithMultipleConfigs(t *testing.T) {
	ctrl := gomock.NewController(t)
	remoteClient := config.NewMockRemoteClient(ctrl)
	mrfClient := config.NewMockRemoteClient(ctrl)
	prioritySampler := NewMockprioritySampler(ctrl)
	errorsSampler := NewMockerrorsSampler(ctrl)
	rareSampler := NewMockrareSampler(ctrl)
	pkglog.SetupLogger(pkglog.Default(), "debug")

	agentConfig := config.AgentConfig{
		RemoteConfigClient:    remoteClient,
		MRFRemoteConfigClient: mrfClient,
		DebugServerPort:       1,
	}
	h := New(&agentConfig, prioritySampler, rareSampler, errorsSampler)

	// Test with multiple configs, first one should take precedence
	enableAPM1 := true
	enableAPM2 := false
	mrfConfig1 := map[string]interface{}{
		"failover_apm": &enableAPM1,
	}
	mrfConfig2 := map[string]interface{}{
		"failover_apm": &enableAPM2,
	}
	raw1, _ := json.Marshal(mrfConfig1)
	raw2, _ := json.Marshal(mrfConfig2)
	config1 := state.RawConfig{Config: raw1}
	config2 := state.RawConfig{Config: raw2}

	h.mrfUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_FAILOVER/config1": config1,
		"datadog/2/AGENT_FAILOVER/config2": config2,
	}, applyEmpty)
	assert.True(t, h.agentConfig.MRFFailoverAPM())
}

// --- onSemanticCoreUpdate tests ---

const semanticTestJSON = `{"version":"test-1.0","metadata":{"content_hash":"hash-a"},"concepts":{"db.statement":{"canonical":"db.statement","fallbacks":[{"name":"db.statement","provider":"datadog","type":"string"}]}}}`

// newSemanticTestHandler returns a handler with minimal sampler mocks for
// exercising onSemanticCoreUpdate. Tests that mutate the global registry must
// call restoreEmbeddedRegistry(t) before calling this function.
func newSemanticTestHandler(t *testing.T) *RemoteConfigHandler {
	t.Helper()
	ctrl := gomock.NewController(t)
	remoteClient := config.NewMockRemoteClient(ctrl)
	agentConfig := config.AgentConfig{
		RemoteConfigClient:              remoteClient,
		DebugServerPort:                 1,
		RemoteConfigAPMSemanticsEnabled: true,
	}
	pkglog.SetupLogger(pkglog.Default(), "debug")
	return New(&agentConfig, NewMockprioritySampler(ctrl), NewMockrareSampler(ctrl), NewMockerrorsSampler(ctrl))
}

// captureStatuses returns a callback that records ApplyStatus per cfgPath.
func captureStatuses() (map[string]state.ApplyStatus, func(string, state.ApplyStatus)) {
	got := make(map[string]state.ApplyStatus)
	cb := func(p string, s state.ApplyStatus) { got[p] = s }
	return got, cb
}

func TestOnSemanticCoreUpdate_ValidConfig(t *testing.T) {
	restoreEmbeddedRegistry(t)
	h := newSemanticTestHandler(t)

	statuses, cb := captureStatuses()
	h.onSemanticCoreUpdate(map[string]state.RawConfig{
		"datadog/2/APM_SEMANTIC_CORE_DD/cfgA/config": {Config: []byte(semanticTestJSON)},
	}, cb)

	assert.Equal(t, "test-1.0", semantics.DefaultRegistry().Version())
	assert.Equal(t, state.ApplyStateAcknowledged, statuses["datadog/2/APM_SEMANTIC_CORE_DD/cfgA/config"].State)
}

func TestOnSemanticCoreUpdate_MalformedJSON(t *testing.T) {
	restoreEmbeddedRegistry(t)
	h := newSemanticTestHandler(t)
	beforeVersion := semantics.DefaultRegistry().Version()

	statuses, cb := captureStatuses()
	h.onSemanticCoreUpdate(map[string]state.RawConfig{
		"datadog/2/APM_SEMANTIC_CORE_DD/bad/config": {Config: []byte("not valid json")},
	}, cb)

	assert.Equal(t, beforeVersion, semantics.DefaultRegistry().Version(), "registry must not change on parse failure")
	assert.Equal(t, state.ApplyStateError, statuses["datadog/2/APM_SEMANTIC_CORE_DD/bad/config"].State)
	assert.NotEmpty(t, statuses["datadog/2/APM_SEMANTIC_CORE_DD/bad/config"].Error)
}

func TestOnSemanticCoreUpdate_EmptyConcepts(t *testing.T) {
	restoreEmbeddedRegistry(t)
	h := newSemanticTestHandler(t)
	beforeVersion := semantics.DefaultRegistry().Version()

	statuses, cb := captureStatuses()
	h.onSemanticCoreUpdate(map[string]state.RawConfig{
		"datadog/2/APM_SEMANTIC_CORE_DD/empty/config": {Config: []byte(`{"version":"x","metadata":{"content_hash":"hash-a"},"concepts":{}}`)},
	}, cb)

	assert.Equal(t, beforeVersion, semantics.DefaultRegistry().Version(), "registry must not change on empty concepts")
	assert.Equal(t, state.ApplyStateError, statuses["datadog/2/APM_SEMANTIC_CORE_DD/empty/config"].State)
}

func TestOnSemanticCoreUpdate_MixedBatch(t *testing.T) {
	restoreEmbeddedRegistry(t)
	h := newSemanticTestHandler(t)

	statuses, cb := captureStatuses()
	h.onSemanticCoreUpdate(map[string]state.RawConfig{
		"datadog/2/APM_SEMANTIC_CORE_DD/aaa-bad/config":  {Config: []byte("malformed")},
		"datadog/2/APM_SEMANTIC_CORE_DD/zzz-good/config": {Config: []byte(semanticTestJSON)},
	}, cb)

	assert.Equal(t, "test-1.0", semantics.DefaultRegistry().Version(), "valid config must be applied")
	assert.Equal(t, state.ApplyStateError, statuses["datadog/2/APM_SEMANTIC_CORE_DD/aaa-bad/config"].State)
	assert.Equal(t, state.ApplyStateAcknowledged, statuses["datadog/2/APM_SEMANTIC_CORE_DD/zzz-good/config"].State)
}

func TestOnSemanticCoreUpdate_MultipleValidConfigs_LastWins(t *testing.T) {
	restoreEmbeddedRegistry(t)
	h := newSemanticTestHandler(t)

	// Two valid configs with different version strings + content. lex-last wins.
	cfgEarly := `{"version":"early","metadata":{"content_hash":"hash-early"},"concepts":{"db.statement":{"canonical":"db.statement","fallbacks":[{"name":"db.statement","provider":"datadog","type":"string"}]}}}`
	cfgLate := `{"version":"late","metadata":{"content_hash":"hash-late"},"concepts":{"http.method":{"canonical":"http.method","fallbacks":[{"name":"http.method","provider":"otel","type":"string"}]}}}`

	statuses, cb := captureStatuses()
	h.onSemanticCoreUpdate(map[string]state.RawConfig{
		"datadog/2/APM_SEMANTIC_CORE_DD/aaa-early/config": {Config: []byte(cfgEarly)},
		"datadog/2/APM_SEMANTIC_CORE_DD/zzz-late/config":  {Config: []byte(cfgLate)},
	}, cb)

	// lex-last (zzz-late) wins — no merging.
	assert.Equal(t, "late", semantics.DefaultRegistry().Version())
	assert.NotNil(t, semantics.DefaultRegistry().GetAttributePrecedence(semantics.ConceptHTTPMethod))
	assert.Nil(t, semantics.DefaultRegistry().GetAttributePrecedence(semantics.ConceptDBStatement), "early config must NOT be merged in")
	// Both paths Ack'd (they validated; we just only applied one).
	assert.Equal(t, state.ApplyStateAcknowledged, statuses["datadog/2/APM_SEMANTIC_CORE_DD/aaa-early/config"].State)
	assert.Equal(t, state.ApplyStateAcknowledged, statuses["datadog/2/APM_SEMANTIC_CORE_DD/zzz-late/config"].State)
}

func TestOnSemanticCoreUpdate_AllErrors(t *testing.T) {
	restoreEmbeddedRegistry(t)
	h := newSemanticTestHandler(t)
	beforeVersion := semantics.DefaultRegistry().Version()

	statuses, cb := captureStatuses()
	h.onSemanticCoreUpdate(map[string]state.RawConfig{
		"datadog/2/APM_SEMANTIC_CORE_DD/a/config": {Config: []byte("not json")},
		"datadog/2/APM_SEMANTIC_CORE_DD/b/config": {Config: []byte(`{"version":"x","metadata":{"content_hash":"hash-a"},"concepts":{}}`)},
	}, cb)

	assert.Equal(t, beforeVersion, semantics.DefaultRegistry().Version())
	assert.Equal(t, state.ApplyStateError, statuses["datadog/2/APM_SEMANTIC_CORE_DD/a/config"].State)
	assert.Equal(t, state.ApplyStateError, statuses["datadog/2/APM_SEMANTIC_CORE_DD/b/config"].State)
}

// TestOnSemanticCoreUpdate_SameHashNoOp verifies that a second push with the
// same metadata.content_hash as the live registry is detected as a no-op
// (skips the UpdateRegistry call) but is still acknowledged, even when
// Version() differs — content_hash is content-bound and version is not.
func TestOnSemanticCoreUpdate_SameHashNoOp(t *testing.T) {
	restoreEmbeddedRegistry(t)
	h := newSemanticTestHandler(t)

	// Use a custom marker as the live registry so we can observe whether the
	// second push replaced it. UpdateRegistry replaces the live one with this
	// one carrying a sentinel concept (peer.service mapped to x.sentinel).
	const liveJSON = `{"version":"sentinel-1.0","metadata":{"content_hash":"hash-sentinel"},"concepts":{"peer.service":{"canonical":"peer.service","fallbacks":[{"name":"x.sentinel","provider":"datadog","type":"string"}]}}}`
	liveReg, err := semantics.NewRegistryFromJSON([]byte(liveJSON))
	require.NoError(t, err)
	semantics.UpdateRegistry(liveReg)

	// Push a payload with a DIFFERENT version but the SAME content_hash. The
	// handler must NOT swap the registry — the sentinel concept must still be
	// reachable after the push, even though the concepts in this payload
	// differ (the hash is trusted, not recomputed).
	const sameHashDifferentVersion = `{"version":"sentinel-1.1","metadata":{"content_hash":"hash-sentinel"},"concepts":{"db.statement":{"canonical":"db.statement","fallbacks":[{"name":"db.statement","provider":"datadog","type":"string"}]}}}`
	statuses, cb := captureStatuses()
	h.onSemanticCoreUpdate(map[string]state.RawConfig{
		"datadog/2/APM_SEMANTIC_CORE_DD/cfg/config": {Config: []byte(sameHashDifferentVersion)},
	}, cb)

	// Sentinel concept survived: the swap was skipped.
	assert.NotNil(t, semantics.DefaultRegistry().GetAttributePrecedence(semantics.ConceptPeerService))
	assert.Nil(t, semantics.DefaultRegistry().GetAttributePrecedence(semantics.ConceptDBStatement),
		"db.statement must not be present — the second push was supposed to be a no-op")
	assert.Equal(t, "sentinel-1.0", semantics.DefaultRegistry().Version())
	// Even though we skipped the swap, the cfgPath is still acknowledged: the
	// payload was valid, we just decided we didn't need to apply it.
	assert.Equal(t, state.ApplyStateAcknowledged, statuses["datadog/2/APM_SEMANTIC_CORE_DD/cfg/config"].State)
}

// TestOnSemanticCoreUpdate_EmptyUpdatesWhileEmbedded verifies that an empty
// update is a no-op when the embedded registry is already live (i.e. there is
// nothing to revert).
func TestOnSemanticCoreUpdate_EmptyUpdatesWhileEmbedded(t *testing.T) {
	restoreEmbeddedRegistry(t)
	h := newSemanticTestHandler(t)
	embedded, err := semantics.NewEmbeddedRegistry()
	require.NoError(t, err)
	semantics.UpdateRegistry(embedded)
	beforeVersion := semantics.DefaultRegistry().Version()

	called := 0
	h.onSemanticCoreUpdate(map[string]state.RawConfig{}, func(string, state.ApplyStatus) { called++ })

	assert.Equal(t, beforeVersion, semantics.DefaultRegistry().Version(), "registry must remain on the embedded version")
	assert.Equal(t, 0, called, "no applyStateCallback invocations on empty updates")
}

// TestOnSemanticCoreUpdate_EmptyUpdatesRevertToEmbedded verifies that an empty
// updates map after a prior RC-applied registry reverts back to the embedded
// mappings, so a backend untargeting/rollback is reflected without an agent
// restart.
func TestOnSemanticCoreUpdate_EmptyUpdatesRevertToEmbedded(t *testing.T) {
	restoreEmbeddedRegistry(t)
	h := newSemanticTestHandler(t)
	embedded, err := semantics.NewEmbeddedRegistry()
	require.NoError(t, err)

	// First, push a custom RC registry so that the live registry differs from embedded.
	_, cb := captureStatuses()
	h.onSemanticCoreUpdate(map[string]state.RawConfig{
		"datadog/2/APM_SEMANTIC_CORE_DD/cfg/config": {Config: []byte(semanticTestJSON)},
	}, cb)
	require.Equal(t, "test-1.0", semantics.DefaultRegistry().Version())

	// Now deliver an empty update (RC removed the config or untargeted us).
	// The handler must revert to the embedded registry.
	called := 0
	h.onSemanticCoreUpdate(map[string]state.RawConfig{}, func(string, state.ApplyStatus) { called++ })

	assert.Equal(t, embedded.Version(), semantics.DefaultRegistry().Version(), "live registry must be the embedded version after revert")
	assert.Equal(t, 0, called, "no applyStateCallback invocations on empty updates (no cfgPaths to ack)")
}
