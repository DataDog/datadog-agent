// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package remoteconfighandler

import (
	"encoding/json"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state/products/apmsampling"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestStart(t *testing.T) {
	ctrl := gomock.NewController(t)
	remoteClient := NewMockRemoteClient(ctrl)
	agentConfig := config.AgentConfig{RemoteSamplingClient: remoteClient}
	prioritySampler := NewMockprioritySampler(ctrl)
	errorsSampler := NewMockerrorsSampler(ctrl)
	rareSampler := NewMockrareSampler(ctrl)

	h := New(&agentConfig, prioritySampler, rareSampler, errorsSampler)

	remoteClient.EXPECT().RegisterAPMUpdate(gomock.Any()).Times(1)
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

	agentConfig := config.AgentConfig{RemoteSamplingClient: remoteClient, TargetTPS: 41, ErrorTPS: 41, RareSamplerEnabled: true}
	h := New(&agentConfig, prioritySampler, rareSampler, errorsSampler)

	payload := apmsampling.SamplerConfig{
		AllEnvs: apmsampling.SamplerEnvConfig{
			PrioritySamplerTargetTPS: floatPointer(42),
		},
	}

	raw, _ := json.Marshal(payload)
	config := state.APMSamplingConfig{
		Config: raw,
	}

	prioritySampler.EXPECT().UpdateTargetTPS(float64(42)).Times(1)
	errorsSampler.EXPECT().UpdateTargetTPS(float64(41)).Times(1)
	rareSampler.EXPECT().SetEnabled(true).Times(1)

	h.onUpdate(map[string]state.APMSamplingConfig{"datadog/2/APM_SAMPLING/samplerconfig/config": config})

	ctrl.Finish()
}

func TestErrorsSampler(t *testing.T) {
	ctrl := gomock.NewController(t)
	remoteClient := NewMockRemoteClient(ctrl)
	prioritySampler := NewMockprioritySampler(ctrl)
	errorsSampler := NewMockerrorsSampler(ctrl)
	rareSampler := NewMockrareSampler(ctrl)

	agentConfig := config.AgentConfig{RemoteSamplingClient: remoteClient, TargetTPS: 41, ErrorTPS: 41, RareSamplerEnabled: true}
	h := New(&agentConfig, prioritySampler, rareSampler, errorsSampler)

	payload := apmsampling.SamplerConfig{
		AllEnvs: apmsampling.SamplerEnvConfig{
			ErrorsSamplerTargetTPS: floatPointer(42),
		},
	}

	raw, _ := json.Marshal(payload)
	config := state.APMSamplingConfig{
		Config: raw,
	}

	prioritySampler.EXPECT().UpdateTargetTPS(float64(41)).Times(1)
	errorsSampler.EXPECT().UpdateTargetTPS(float64(42)).Times(1)
	rareSampler.EXPECT().SetEnabled(true).Times(1)

	h.onUpdate(map[string]state.APMSamplingConfig{"datadog/2/APM_SAMPLING/samplerconfig/config": config})

	ctrl.Finish()
}

func TestRareSampler(t *testing.T) {
	ctrl := gomock.NewController(t)
	remoteClient := NewMockRemoteClient(ctrl)
	prioritySampler := NewMockprioritySampler(ctrl)
	errorsSampler := NewMockerrorsSampler(ctrl)
	rareSampler := NewMockrareSampler(ctrl)

	agentConfig := config.AgentConfig{RemoteSamplingClient: remoteClient, TargetTPS: 41, ErrorTPS: 41, RareSamplerEnabled: true}
	h := New(&agentConfig, prioritySampler, rareSampler, errorsSampler)

	payload := apmsampling.SamplerConfig{
		AllEnvs: apmsampling.SamplerEnvConfig{
			RareSamplerEnabled: boolPointer(false),
		},
	}

	raw, _ := json.Marshal(payload)
	config := state.APMSamplingConfig{
		Config: raw,
	}

	prioritySampler.EXPECT().UpdateTargetTPS(float64(41)).Times(1)
	errorsSampler.EXPECT().UpdateTargetTPS(float64(41)).Times(1)
	rareSampler.EXPECT().SetEnabled(false).Times(1)

	h.onUpdate(map[string]state.APMSamplingConfig{"datadog/2/APM_SAMPLING/samplerconfig/config": config})

	ctrl.Finish()
}

func TestEnvPrecedence(t *testing.T) {
	ctrl := gomock.NewController(t)
	remoteClient := NewMockRemoteClient(ctrl)
	prioritySampler := NewMockprioritySampler(ctrl)
	errorsSampler := NewMockerrorsSampler(ctrl)
	rareSampler := NewMockrareSampler(ctrl)

	agentConfig := config.AgentConfig{RemoteSamplingClient: remoteClient, TargetTPS: 41, ErrorTPS: 41, RareSamplerEnabled: true, DefaultEnv: "agent-env"}
	h := New(&agentConfig, prioritySampler, rareSampler, errorsSampler)

	payload := apmsampling.SamplerConfig{
		AllEnvs: apmsampling.SamplerEnvConfig{
			PrioritySamplerTargetTPS: floatPointer(42),
			ErrorsSamplerTargetTPS:   floatPointer(42),
			RareSamplerEnabled:       boolPointer(true),
		},
		ByEnv: []apmsampling.EnvAndConfig{{
			Env: "agent-env",
			Config: apmsampling.SamplerEnvConfig{
				PrioritySamplerTargetTPS: floatPointer(43),
				ErrorsSamplerTargetTPS:   floatPointer(43),
				RareSamplerEnabled:       boolPointer(false),
			},
		}},
	}

	raw, _ := json.Marshal(payload)
	config := state.APMSamplingConfig{
		Config: raw,
	}

	prioritySampler.EXPECT().UpdateTargetTPS(float64(43)).Times(1)
	errorsSampler.EXPECT().UpdateTargetTPS(float64(43)).Times(1)
	rareSampler.EXPECT().SetEnabled(false).Times(1)

	h.onUpdate(map[string]state.APMSamplingConfig{"datadog/2/APM_SAMPLING/samplerconfig/config": config})

	ctrl.Finish()
}

func floatPointer(f float64) *float64 {
	return &f
}

func boolPointer(b bool) *bool {
	return &b
}
