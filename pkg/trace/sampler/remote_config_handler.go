// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state/products/apmsampling"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

// ApmRemoteConfigHandler holds pointers to samplers that need to be updated when APM remote config changes
type ApmRemoteConfigHandler struct {
	remoteClient    config.RemoteClient
	conf            *config.AgentConfig
	prioritySampler *PrioritySampler
	errorsSampler   *ErrorsSampler
}

func NewApmRemoteConfigHandler(conf *config.AgentConfig, prioritySampler *PrioritySampler, errorSampler *ErrorsSampler) *ApmRemoteConfigHandler {
	if conf.RemoteSamplingClient == nil {
		return nil
	}

	prioritySampler.EnableRemoteRates(conf.MaxRemoteTPS, conf.AgentVersion)

	return &ApmRemoteConfigHandler{
		remoteClient:    conf.RemoteSamplingClient,
		conf:            conf,
		prioritySampler: prioritySampler,
		errorsSampler:   errorSampler,
	}
}

func (a *ApmRemoteConfigHandler) Start() {
	if a == nil {
		return
	}

	a.remoteClient.Start()
	a.remoteClient.RegisterAPMUpdate(a.onUpdate)
}

func (a *ApmRemoteConfigHandler) onUpdate(update map[string]state.APMSamplingConfig) {
	a.prioritySampler.remoteRates.update(update)
	a.updateRareSamplerConfig(update)
	a.updateErrorsSamplerConfig(update)
}

func (a *ApmRemoteConfigHandler) updateRareSamplerConfig(update map[string]state.APMSamplingConfig) {
	for _, conf := range update {
		// We expect the `update` map to contain only one entry for now
		switch conf.Config.RareSamplerConfig {
		case apmsampling.RareSamplerConfigEnabled:
			a.conf.RareSamplerDisabled = false
		case apmsampling.RareSamplerConfigDisabled:
			a.conf.RareSamplerDisabled = true
		}
	}
}

func (a *ApmRemoteConfigHandler) updateErrorsSamplerConfig(update map[string]state.APMSamplingConfig) {
	for _, conf := range update {
		// We expect the `update` map to contain only one entry for now
		if conf.Config.ErrorsSamplerConfig == nil {
			continue
		}

		a.errorsSampler.updateTargetTPS(conf.Config.ErrorsSamplerConfig.TargetTPS)
	}
}
