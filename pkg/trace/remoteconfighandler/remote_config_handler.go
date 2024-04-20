// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package remoteconfighandler holds the logic responsible for updating the samplers when the remote configuration changes.
package remoteconfighandler

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state/products/apmsampling"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/cihub/seelog"
	"github.com/davecgh/go-spew/spew"
)

type prioritySampler interface {
	UpdateTargetTPS(targetTPS float64)
}

type errorsSampler interface {
	UpdateTargetTPS(targetTPS float64)
}

type rareSampler interface {
	SetEnabled(enabled bool)
}

// RemoteConfigHandler holds pointers to samplers that need to be updated when APM remote config changes
type RemoteConfigHandler struct {
	remoteClient                  config.RemoteClient
	prioritySampler               prioritySampler
	errorsSampler                 errorsSampler
	rareSampler                   rareSampler
	agentConfig                   *config.AgentConfig
	configState                   *state.AgentConfigState
	configSetEndpointFormatString string
}

// New creates a new RemoteConfigHandler
func New(conf *config.AgentConfig, prioritySampler prioritySampler, rareSampler rareSampler, errorsSampler errorsSampler) *RemoteConfigHandler {
	if conf.RemoteConfigClient == nil {
		return nil
	}

	level, err := pkglog.GetLogLevel()
	if err != nil {
		log.Errorf("couldn't get the default log level: %s", err)
		return nil
	}

	return &RemoteConfigHandler{
		remoteClient:    conf.RemoteConfigClient,
		prioritySampler: prioritySampler,
		rareSampler:     rareSampler,
		errorsSampler:   errorsSampler,
		agentConfig:     conf,
		configState: &state.AgentConfigState{
			FallbackLogLevel: level.String(),
		},
		configSetEndpointFormatString: fmt.Sprintf(
			"http://%s/config/set?log_level=%%s", net.JoinHostPort(conf.ReceiverHost, strconv.Itoa(conf.ReceiverPort)),
		),
	}
}

// Start starts the remote config handler
func (h *RemoteConfigHandler) Start() {
	if h == nil {
		return
	}

	h.remoteClient.Start()
	h.remoteClient.Subscribe(state.ProductAPMSampling, h.onUpdate)
	h.remoteClient.Subscribe(state.ProductAgentConfig, h.onAgentConfigUpdate)
}

func (h *RemoteConfigHandler) onAgentConfigUpdate(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	mergedConfig, err := state.MergeRCAgentConfig(h.remoteClient.UpdateApplyStatus, updates)
	if err != nil {
		log.Debugf("couldn't merge the agent config from remote configuration: %s", err)
		return
	}

	if len(mergedConfig.LogLevel) > 0 {
		// Get the current log level
		var newFallback seelog.LogLevel
		newFallback, err = pkglog.GetLogLevel()
		if err == nil {
			h.configState.FallbackLogLevel = newFallback.String()
			var resp *http.Response
			resp, err = http.Post(fmt.Sprintf(h.configSetEndpointFormatString, mergedConfig.LogLevel), "", nil)
			if err == nil {
				resp.Body.Close()
				h.configState.LatestLogLevel = mergedConfig.LogLevel
				pkglog.Infof("Changing log level of the trace-agent to %s through remote config", mergedConfig.LogLevel)
			}
		}
	} else {
		var currentLogLevel seelog.LogLevel
		currentLogLevel, err = pkglog.GetLogLevel()
		if err == nil && currentLogLevel.String() == h.configState.LatestLogLevel {
			pkglog.Infof("Removing remote-config log level override of the trace-agent, falling back to %s", h.configState.FallbackLogLevel)
			var resp *http.Response
			resp, err = http.Post(fmt.Sprintf(h.configSetEndpointFormatString, h.configState.FallbackLogLevel), "", nil)
			if err == nil {
				resp.Body.Close()
			}
		}
	}

	if err != nil {
		log.Errorf("couldn't apply the remote configuration agent config: %s", err)
	}

	// Apply the new status to all configs
	for cfgPath := range updates {
		if err == nil {
			applyStateCallback(cfgPath, state.ApplyStatus{State: state.ApplyStateAcknowledged})
		} else {
			applyStateCallback(cfgPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
		}
	}
}

func (h *RemoteConfigHandler) onUpdate(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) { //nolint:revive // TODO fix revive unused-parameter
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

	log.Debugf("updating samplers with remote configuration: %v", spew.Sdump(samplerconfigPayload))
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
