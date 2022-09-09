// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

// ApmRemoteConfigHandler holds pointers to samplers that need to be updated when APM remote config changes
type ApmRemoteConfigHandler struct {
	remoteClient config.RemoteClient

	prioritySampler *PrioritySampler
}

func NewApmRemoteConfigHandler(conf *config.AgentConfig, prioritySampler *PrioritySampler) *ApmRemoteConfigHandler {
	if conf.RemoteSamplingClient == nil {
		return nil
	}

	prioritySampler.EnableRemoteRates(conf.MaxRemoteTPS, conf.AgentVersion)

	return &ApmRemoteConfigHandler{
		remoteClient:    conf.RemoteSamplingClient,
		prioritySampler: prioritySampler,
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
}
