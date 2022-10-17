// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfighandler

import (
	"fmt"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state/products/apmsampling"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
)

type apmsamplingRemoteConfigID string

const (
	dynamicRatesByService     apmsamplingRemoteConfigID = "dynamic_rates"
	userDefinedRatesByService apmsamplingRemoteConfigID = "user_rates"
	samplerconfig             apmsamplingRemoteConfigID = "samplerconfig"
)

type prioritySampler interface {
	UpdateTargetTPS(targetTPS float64)
	UpdateRemoteRates(updates []sampler.RemoteRateUpdate)
}

type errorsSampler interface {
	UpdateTargetTPS(targetTPS float64)
}

type rareSampler interface {
	SetEnabled(enabled bool)
}

// RemoteConfigHandler holds pointers to samplers that need to be updated when APM remote config changes
type RemoteConfigHandler struct {
	remoteClient          config.RemoteClient
	prioritySampler       prioritySampler
	errorsSampler         errorsSampler
	rareSampler           rareSampler
	agentConfig           *config.AgentConfig
	remoteConfigPathRegex *regexp.Regexp
}

func New(conf *config.AgentConfig, prioritySampler prioritySampler, rareSampler rareSampler, errorsSampler errorsSampler) *RemoteConfigHandler {
	if conf.RemoteSamplingClient == nil {
		return nil
	}

	// paths are in the form datadog/<org ID>/<product name>/<config ID>/config
	r := regexp.MustCompile(`datadog\/\d+\/APM_SAMPLING\/(.+)\/config`)

	return &RemoteConfigHandler{
		remoteClient:          conf.RemoteSamplingClient,
		prioritySampler:       prioritySampler,
		rareSampler:           rareSampler,
		errorsSampler:         errorsSampler,
		agentConfig:           conf,
		remoteConfigPathRegex: r,
	}
}

func (h *RemoteConfigHandler) Start() {
	if h == nil {
		return
	}

	h.remoteClient.Start()
	h.remoteClient.RegisterAPMUpdate(h.onUpdate)
}

func (h *RemoteConfigHandler) onUpdate(update map[string]state.APMSamplingConfig) {
	remoteRateUpdates, samplerconfigPayload, err := h.extractUpdatePayloads(update)
	if err != nil {
		log.Error(err)
		return
	}

	if len(remoteRateUpdates) != 0 {
		h.prioritySampler.UpdateRemoteRates(remoteRateUpdates)
	}

	if samplerconfigPayload != nil {
		h.updateSamplers(*samplerconfigPayload)
	}
}

func (h *RemoteConfigHandler) extractUpdatePayloads(update map[string]state.APMSamplingConfig) ([]sampler.RemoteRateUpdate, *apmsampling.SamplerConfig, error) {
	var remoteRateUpdates []sampler.RemoteRateUpdate
	var samplerconfigPayload *apmsampling.SamplerConfig
	for path, conf := range update {
		configID, err := h.configPathToID(path)
		if err != nil {
			return []sampler.RemoteRateUpdate{}, nil, err
		}

		switch configID {
		case dynamicRatesByService, userDefinedRatesByService:
			var payload apmsampling.APMSampling
			_, err := payload.UnmarshalMsg(conf.Config)
			if err != nil {
				return []sampler.RemoteRateUpdate{}, nil, fmt.Errorf("failed to unmarshal APMSampling remote config envPayload: %w", err)
			}
			remoteRateUpdates = append(remoteRateUpdates, sampler.RemoteRateUpdate{Version: conf.Metadata.Version, Config: payload})
		case samplerconfig:
			samplerconfigPayload = &apmsampling.SamplerConfig{}
			_, err := samplerconfigPayload.UnmarshalMsg(conf.Config)
			if err != nil {
				return []sampler.RemoteRateUpdate{}, nil, fmt.Errorf("failed to unmarshal SamplerConfig remote config envPayload: %w", err)
			}
		}
	}
	return remoteRateUpdates, samplerconfigPayload, nil
}

func (h *RemoteConfigHandler) configPathToID(path string) (apmsamplingRemoteConfigID, error) {
	pathMatch := h.remoteConfigPathRegex.FindStringSubmatch(path)
	if pathMatch == nil {
		return "", fmt.Errorf("failed to match remote config path %s", path)
	}

	switch pathMatch[1] {
	case string(dynamicRatesByService):
		return dynamicRatesByService, nil
	case string(userDefinedRatesByService):
		return userDefinedRatesByService, nil
	case string(samplerconfig):
		return samplerconfig, nil
	default:
		return "", fmt.Errorf("unexpected remote config ID: %s", pathMatch[1])
	}
}

func (h *RemoteConfigHandler) updateSamplers(config apmsampling.SamplerConfig) {
	var confForEnv *apmsampling.SamplerEnvConfig
	for _, envAndConfig := range config.ByEnv {
		if envAndConfig.Env == h.agentConfig.DefaultEnv {
			confForEnv = &envAndConfig.Config
		}
	}

	var prioritySamplerTargetTPS float64
	if confForEnv != nil && confForEnv.PrioritySamplerTargetTPS != nil {
		prioritySamplerTargetTPS = *confForEnv.PrioritySamplerTargetTPS
	} else if config.AllEnvs.PrioritySamplerTargetTPS != nil {
		prioritySamplerTargetTPS = *config.AllEnvs.PrioritySamplerTargetTPS
	} else {
		prioritySamplerTargetTPS = h.agentConfig.TargetTPS
	}
	h.prioritySampler.UpdateTargetTPS(prioritySamplerTargetTPS)

	var errorsSamplerTargetTPS float64
	if confForEnv != nil && confForEnv.ErrorsSamplerTargetTPS != nil {
		errorsSamplerTargetTPS = *confForEnv.ErrorsSamplerTargetTPS
	} else if config.AllEnvs.ErrorsSamplerTargetTPS != nil {
		errorsSamplerTargetTPS = *config.AllEnvs.ErrorsSamplerTargetTPS
	} else {
		errorsSamplerTargetTPS = h.agentConfig.ErrorTPS
	}
	h.errorsSampler.UpdateTargetTPS(errorsSamplerTargetTPS)

	var rareSamplerEnabled bool
	if confForEnv != nil && confForEnv.RareSamplerEnabled != nil {
		rareSamplerEnabled = *confForEnv.RareSamplerEnabled
	} else if config.AllEnvs.RareSamplerEnabled != nil {
		rareSamplerEnabled = *config.AllEnvs.RareSamplerEnabled
	} else {
		rareSamplerEnabled = h.agentConfig.RareSamplerEnabled
	}
	h.rareSampler.SetEnabled(rareSamplerEnabled)
}
