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

// APMRemoteConfigHandler holds pointers to samplers that need to be updated when APM remote config changes
type APMRemoteConfigHandler struct {
	remoteClient    config.RemoteClient
	conf            *config.AgentConfig
	prioritySampler *PrioritySampler
	errorsSampler   *ErrorsSampler
}

func NewAPMRemoteConfigHandler(conf *config.AgentConfig, prioritySampler *PrioritySampler, errorSampler *ErrorsSampler) *APMRemoteConfigHandler {
	if conf.RemoteSamplingClient == nil {
		return nil
	}

	prioritySampler.enableRemoteRates(conf.MaxRemoteTPS, conf.AgentVersion)

	return &APMRemoteConfigHandler{
		remoteClient:    conf.RemoteSamplingClient,
		conf:            conf,
		prioritySampler: prioritySampler,
		errorsSampler:   errorSampler,
	}
}

func (a *APMRemoteConfigHandler) Start() {
	if a == nil {
		return
	}

	a.remoteClient.Start()
	a.remoteClient.RegisterAPMUpdate(a.onUpdate)
}

func (a *APMRemoteConfigHandler) onUpdate(update map[string]state.APMSamplingConfig) {
	a.prioritySampler.remoteRates.update(update)
	a.updateRareSamplerConfig(update)
	a.updateErrorsSamplerConfig(update)
}

func (a *APMRemoteConfigHandler) updateRareSamplerConfig(update map[string]state.APMSamplingConfig) {
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

func (a *APMRemoteConfigHandler) updateErrorsSamplerConfig(update map[string]state.APMSamplingConfig) {
	for _, conf := range update {
		// We expect the `update` map to contain only one entry for now
		if conf.Config.ErrorsSamplerConfig == nil {
			continue
		}

		a.errorsSampler.updateTargetTPS(conf.Config.ErrorsSamplerConfig.TargetTPS)
	}
}
