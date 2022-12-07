// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfighandler

import (
	"encoding/json"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state/products/apmsampling"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
)

type samplerWithTPS interface {
	UpdateTargetTPS(targetTPS float64, remotelyConfigured bool)
}

type rareSampler interface {
	Configure(enabled bool, remotelyConfigured bool)
}

// RemoteConfigHandler holds pointers to samplers that need to be updated when APM remote config changes
type RemoteConfigHandler struct {
	remoteClient    config.RemoteClient
	prioritySampler samplerWithTPS
	errorsSampler   samplerWithTPS
	rareSampler     rareSampler
	agentConfig     *config.AgentConfig
}

func New(conf *config.AgentConfig, prioritySampler samplerWithTPS, rareSampler rareSampler, errorsSampler samplerWithTPS) *RemoteConfigHandler {
	if conf.RemoteSamplingClient == nil {
		return nil
	}

	return &RemoteConfigHandler{
		remoteClient:    conf.RemoteSamplingClient,
		prioritySampler: prioritySampler,
		rareSampler:     rareSampler,
		errorsSampler:   errorsSampler,
		agentConfig:     conf,
	}
}

func (h *RemoteConfigHandler) Start() {
	if h == nil {
		return
	}

	h.remoteClient.Start()
	h.remoteClient.RegisterAPMUpdate(h.onUpdate)
}

func (h *RemoteConfigHandler) Stop() {
	if h == nil {
		return
	}

	h.remoteClient.Close()
}

func (h *RemoteConfigHandler) onUpdate(update map[string]state.APMSamplingConfig) {
	if len(update) == 0 {
		log.Debugf("no samplers configuration in remote config update payload")
		return
	}

	if len(update) > 1 {
		log.Errorf("samplers remote config payload contains %v configurations, but it should contain at most one", len(update))
		return
	}

	var samplerconfigPayload apmsampling.SamplerConfig
	for _, v := range update {
		err := json.Unmarshal(v.Config, &samplerconfigPayload)
		if err != nil {
			log.Error(err)
			return
		}
	}

	log.Debugf("updating samplers with remote configuration: %v", samplerconfigPayload)
	h.updateSamplers(samplerconfigPayload)
}

func (h *RemoteConfigHandler) updateSamplers(config apmsampling.SamplerConfig) {
	var confForEnv *apmsampling.SamplerEnvConfig
	for _, envAndConfig := range config.ByEnv {
		if envAndConfig.Env == h.agentConfig.DefaultEnv {
			confForEnv = &envAndConfig.Config
		}
	}

	var prioritySamplerTargetTPS float64
	var prioritySamplerRemotelyConfigured bool
	if confForEnv != nil && confForEnv.PrioritySamplerTargetTPS != nil {
		prioritySamplerTargetTPS = *confForEnv.PrioritySamplerTargetTPS
		prioritySamplerRemotelyConfigured = true
	} else if config.AllEnvs.PrioritySamplerTargetTPS != nil {
		prioritySamplerTargetTPS = *config.AllEnvs.PrioritySamplerTargetTPS
		prioritySamplerRemotelyConfigured = true
	} else {
		prioritySamplerTargetTPS = h.agentConfig.TargetTPS
		prioritySamplerRemotelyConfigured = false
	}
	h.prioritySampler.UpdateTargetTPS(prioritySamplerTargetTPS, prioritySamplerRemotelyConfigured)

	var errorsSamplerTargetTPS float64
	var errorsSamplerRemotelyConfigured bool
	if confForEnv != nil && confForEnv.ErrorsSamplerTargetTPS != nil {
		errorsSamplerTargetTPS = *confForEnv.ErrorsSamplerTargetTPS
		errorsSamplerRemotelyConfigured = true
	} else if config.AllEnvs.ErrorsSamplerTargetTPS != nil {
		errorsSamplerTargetTPS = *config.AllEnvs.ErrorsSamplerTargetTPS
		errorsSamplerRemotelyConfigured = true
	} else {
		errorsSamplerTargetTPS = h.agentConfig.ErrorTPS
		errorsSamplerRemotelyConfigured = false
	}
	h.errorsSampler.UpdateTargetTPS(errorsSamplerTargetTPS, errorsSamplerRemotelyConfigured)

	var rareSamplerEnabled bool
	var rareSamplerRemotelyConfigured bool
	if confForEnv != nil && confForEnv.RareSamplerEnabled != nil {
		rareSamplerEnabled = *confForEnv.RareSamplerEnabled
		rareSamplerRemotelyConfigured = true
	} else if config.AllEnvs.RareSamplerEnabled != nil {
		rareSamplerEnabled = *config.AllEnvs.RareSamplerEnabled
		rareSamplerRemotelyConfigured = true
	} else {
		rareSamplerEnabled = h.agentConfig.RareSamplerEnabled
		rareSamplerRemotelyConfigured = false
	}
	h.rareSampler.Configure(rareSamplerEnabled, rareSamplerRemotelyConfigured)
}
