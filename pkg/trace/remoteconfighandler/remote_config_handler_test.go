// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package remoteconfighandler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state/products/apmsampling"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

// nolint: revive
func applyEmpty(_ string, _ state.ApplyStatus) {}

func TestStart(t *testing.T) {
	ctrl := gomock.NewController(t)
	remoteClient := NewMockRemoteClient(ctrl)
	agentConfig := config.AgentConfig{RemoteConfigClient: remoteClient, DebugServerPort: 1}
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

func TestStartNoRemoteClient(t *testing.T) {
	var h *RemoteConfigHandler
	assert.NotPanics(t, h.Start)
}

func TestPrioritySampler(t *testing.T) {
	ctrl := gomock.NewController(t)
	remoteClient := NewMockRemoteClient(ctrl)
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
	remoteClient := NewMockRemoteClient(ctrl)
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
	remoteClient := NewMockRemoteClient(ctrl)
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
	remoteClient := NewMockRemoteClient(ctrl)
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
	remoteClient := NewMockRemoteClient(ctrl)
	prioritySampler := NewMockprioritySampler(ctrl)
	errorsSampler := NewMockerrorsSampler(ctrl)
	rareSampler := NewMockrareSampler(ctrl)

	pkglog.SetupLogger(pkglog.Default(), "debug")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	remoteClient := NewMockRemoteClient(ctrl)
	mrfClient := NewMockRemoteClient(ctrl)
	agentConfig := config.AgentConfig{
		RemoteConfigClient:    remoteClient,
		MRFRemoteConfigClient: mrfClient,
		DebugServerPort:       1,
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
	remoteClient := NewMockRemoteClient(ctrl)
	mrfClient := NewMockRemoteClient(ctrl)
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
	remoteClient := NewMockRemoteClient(ctrl)
	mrfClient := NewMockRemoteClient(ctrl)
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
