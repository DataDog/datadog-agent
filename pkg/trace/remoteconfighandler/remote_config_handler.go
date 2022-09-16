// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfighandler

import (
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
)

// RemoteConfigHandler holds pointers to samplers that need to be updated when APM remote config changes
type RemoteConfigHandler struct {
	remoteClient    config.RemoteClient
	prioritySampler *sampler.PrioritySampler
}

func New(conf *config.AgentConfig, prioritySampler *sampler.PrioritySampler) *RemoteConfigHandler {
	if conf.RemoteSamplingClient == nil {
		return nil
	}

	return &RemoteConfigHandler{
		remoteClient:    conf.RemoteSamplingClient,
		prioritySampler: prioritySampler,
	}
}

func (a *RemoteConfigHandler) Start() {
	if a == nil {
		return
	}

	a.remoteClient.Start()
	a.remoteClient.RegisterAPMUpdate(a.onUpdate)
}

func (a *RemoteConfigHandler) onUpdate(update map[string]state.APMSamplingConfig) {
	a.prioritySampler.UpdateRemoteRates(update)
}
