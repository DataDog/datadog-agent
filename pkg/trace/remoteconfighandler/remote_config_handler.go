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

	if confForEnv != nil && confForEnv.PrioritySamplerTargetTPS != nil {
		h.prioritySampler.UpdateTargetTPS(*confForEnv.PrioritySamplerTargetTPS, true)
	} else if config.AllEnvs.PrioritySamplerTargetTPS != nil {
		h.prioritySampler.UpdateTargetTPS(*config.AllEnvs.PrioritySamplerTargetTPS, true)
	} else {
		h.prioritySampler.UpdateTargetTPS(h.agentConfig.TargetTPS, false)
	}

	if confForEnv != nil && confForEnv.ErrorsSamplerTargetTPS != nil {
		h.errorsSampler.UpdateTargetTPS(*confForEnv.ErrorsSamplerTargetTPS, true)
	} else if config.AllEnvs.ErrorsSamplerTargetTPS != nil {
		h.errorsSampler.UpdateTargetTPS(*config.AllEnvs.ErrorsSamplerTargetTPS, true)
	} else {
		h.errorsSampler.UpdateTargetTPS(h.agentConfig.ErrorTPS, false)
	}

	if confForEnv != nil && confForEnv.RareSamplerEnabled != nil {
		h.rareSampler.Configure(*confForEnv.RareSamplerEnabled, true)
	} else if config.AllEnvs.RareSamplerEnabled != nil {
		h.rareSampler.Configure(*config.AllEnvs.RareSamplerEnabled, true)
	} else {
		h.rareSampler.Configure(h.agentConfig.RareSamplerEnabled, false)
	}
}
