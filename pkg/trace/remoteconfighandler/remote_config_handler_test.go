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

	"github.com/cihub/seelog"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state/products/apmsampling"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

// nolint: revive
func applyEmpty(s string, as state.ApplyStatus) {}

func TestStart(t *testing.T) {
	ctrl := gomock.NewController(t)
	remoteClient := NewMockRemoteClient(ctrl)
	agentConfig := config.AgentConfig{RemoteConfigClient: remoteClient}
	prioritySampler := NewMockprioritySampler(ctrl)
	errorsSampler := NewMockerrorsSampler(ctrl)
	rareSampler := NewMockrareSampler(ctrl)
	pkglog.SetupLogger(seelog.Default, "debug")

	h := New(&agentConfig, prioritySampler, rareSampler, errorsSampler)

	remoteClient.EXPECT().Subscribe(state.ProductAPMSampling, gomock.Any()).Times(1)
	remoteClient.EXPECT().Subscribe(state.ProductAgentConfig, gomock.Any()).Times(1)
	remoteClient.EXPECT().Subscribe(state.ProductAPMTracing, gomock.Any()).Times(1)

	remoteClient.EXPECT().Start().Times(1)

	h.Start()

	ctrl.Finish()
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
	pkglog.SetupLogger(seelog.Default, "debug")

	agentConfig := config.AgentConfig{RemoteConfigClient: remoteClient, TargetTPS: 41, ErrorTPS: 41, RareSamplerEnabled: true}
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

	ctrl.Finish()
}

func TestErrorsSampler(t *testing.T) {
	ctrl := gomock.NewController(t)
	remoteClient := NewMockRemoteClient(ctrl)
	prioritySampler := NewMockprioritySampler(ctrl)
	errorsSampler := NewMockerrorsSampler(ctrl)
	rareSampler := NewMockrareSampler(ctrl)
	pkglog.SetupLogger(seelog.Default, "debug")

	agentConfig := config.AgentConfig{RemoteConfigClient: remoteClient, TargetTPS: 41, ErrorTPS: 41, RareSamplerEnabled: true}
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

	ctrl.Finish()
}

func TestRareSampler(t *testing.T) {
	ctrl := gomock.NewController(t)
	remoteClient := NewMockRemoteClient(ctrl)
	prioritySampler := NewMockprioritySampler(ctrl)
	errorsSampler := NewMockerrorsSampler(ctrl)
	rareSampler := NewMockrareSampler(ctrl)
	pkglog.SetupLogger(seelog.Default, "debug")

	agentConfig := config.AgentConfig{RemoteConfigClient: remoteClient, TargetTPS: 41, ErrorTPS: 41, RareSamplerEnabled: true}
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

	ctrl.Finish()
}

func TestEnvPrecedence(t *testing.T) {
	ctrl := gomock.NewController(t)
	remoteClient := NewMockRemoteClient(ctrl)
	prioritySampler := NewMockprioritySampler(ctrl)
	errorsSampler := NewMockerrorsSampler(ctrl)
	rareSampler := NewMockrareSampler(ctrl)
	pkglog.SetupLogger(seelog.Default, "debug")

	agentConfig := config.AgentConfig{RemoteConfigClient: remoteClient, TargetTPS: 41, ErrorTPS: 41, RareSamplerEnabled: true, DefaultEnv: "agent-env"}
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

	ctrl.Finish()
}

func TestLogLevel(t *testing.T) {
	ctrl := gomock.NewController(t)
	remoteClient := NewMockRemoteClient(ctrl)
	prioritySampler := NewMockprioritySampler(ctrl)
	errorsSampler := NewMockerrorsSampler(ctrl)
	rareSampler := NewMockrareSampler(ctrl)

	pkglog.SetupLogger(seelog.Default, "debug")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()
	port, _ := strconv.Atoi(strings.Split(srv.URL, ":")[2])

	agentConfig := config.AgentConfig{
		RemoteConfigClient: remoteClient,
		DefaultEnv:         "agent-env",
		ReceiverHost:       "127.0.0.1",
		ReceiverPort:       port,
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

	ctrl.Finish()
}
