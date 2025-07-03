// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package remoteconfighandler holds the logic responsible for updating the samplers when the remote configuration changes.
package remoteconfighandler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state/products/apmsampling"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"

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
	client                        config.RemoteClient
	mrfClient                     config.RemoteClient
	prioritySampler               prioritySampler
	errorsSampler                 errorsSampler
	rareSampler                   rareSampler
	agentConfig                   *config.AgentConfig
	configState                   *state.AgentConfigState
	configHTTPClient              *http.Client
	configSetEndpointFormatString string
}

// New creates a new RemoteConfigHandler
func New(conf *config.AgentConfig, prioritySampler prioritySampler, rareSampler rareSampler, errorsSampler errorsSampler) *RemoteConfigHandler {
	if conf.RemoteConfigClient == nil {
		return nil
	}

	if conf.DebugServerPort == 0 {
		log.Errorf("debug server(apm_config.debug.port) was disabled, server is required for remote config, RC is disabled.")
		return nil
	}

	level, err := pkglog.GetLogLevel()
	if err != nil {
		log.Errorf("couldn't get the default log level: %s", err)
		return nil
	}

	return &RemoteConfigHandler{
		client:          conf.RemoteConfigClient,
		mrfClient:       conf.MRFRemoteConfigClient,
		prioritySampler: prioritySampler,
		rareSampler:     rareSampler,
		errorsSampler:   errorsSampler,
		agentConfig:     conf,
		configState: &state.AgentConfigState{
			FallbackLogLevel: level.String(),
		},
		configHTTPClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: conf.IPCTLSClientConfig,
			},
		},
		configSetEndpointFormatString: fmt.Sprintf(
			"https://127.0.0.1:%s/config/set?log_level=%%s", strconv.Itoa(conf.DebugServerPort),
		),
	}
}

// Start starts the remote config handler
func (h *RemoteConfigHandler) Start() {
	if h == nil {
		return
	}

	h.client.Start()
	h.client.Subscribe(state.ProductAPMSampling, h.onUpdate)
	h.client.Subscribe(state.ProductAgentConfig, h.onAgentConfigUpdate)
	if h.mrfClient != nil {
		h.mrfClient.Start()
		h.mrfClient.Subscribe(state.ProductAgentFailover, h.mrfUpdateCallback)
	}
}

// mrfUpdateCallback is the callback function for the AGENT_FAILOVER configs.
// It fetches all the configs targeting the agent and applies the failover_apm settings.
// Note: we only care about APM (failover_apm) configuration, but Logs (failover_logs) and Metrics (failover_metrics)
// may also be present on the MRF configuration. These are handled on the core agent RC.
func (h *RemoteConfigHandler) mrfUpdateCallback(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	var failoverAPM *bool
	var failoverAPMCfgPth string
	for cfgPath, update := range updates {
		mrfUpdate, err := parseMultiRegionFailoverConfig(update.Config)
		if err != nil {
			pkglog.Errorf("Multi-Region Failover update unmarshal failed: %s", err)
			applyStateCallback(cfgPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
			continue
		}

		if mrfUpdate == nil || mrfUpdate.FailoverAPM == nil {
			continue
		}

		if failoverAPM == nil || *mrfUpdate.FailoverAPM {
			failoverAPM = mrfUpdate.FailoverAPM
			failoverAPMCfgPth = cfgPath

			if *mrfUpdate.FailoverAPM {
				break
			}
		}
	}

	h.agentConfig.MRFFailoverAPMRC = failoverAPM
	if failoverAPM != nil {
		pkglog.Infof("Setting `multi_region_failover.failover_apm: %t` through remote config", *failoverAPM)
		applyStateCallback(failoverAPMCfgPth, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}
}

func (h *RemoteConfigHandler) onAgentConfigUpdate(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	mergedConfig, err := state.MergeRCAgentConfig(h.client.UpdateApplyStatus, updates)
	if err != nil {
		log.Debugf("couldn't merge the agent config from remote configuration: %s", err)
		return
	}

	// todo refactor shared code

	if len(mergedConfig.LogLevel) > 0 {
		// Get the current log level
		var newFallback pkglog.LogLevel
		newFallback, err = pkglog.GetLogLevel()
		if err == nil {
			h.configState.FallbackLogLevel = newFallback.String()
			var resp *http.Response
			var req *http.Request
			req, err = h.buildLogLevelRequest(mergedConfig.LogLevel)
			if err != nil {
				return
			}
			resp, err = h.configHTTPClient.Do(req)
			if err == nil {
				resp.Body.Close()
				h.configState.LatestLogLevel = mergedConfig.LogLevel
				pkglog.Infof("Changing log level of the trace-agent to %s through remote config", mergedConfig.LogLevel)
			}
		}
	} else {
		var currentLogLevel pkglog.LogLevel
		currentLogLevel, err = pkglog.GetLogLevel()
		if err == nil && currentLogLevel.String() == h.configState.LatestLogLevel {
			pkglog.Infof("Removing remote-config log level override of the trace-agent, falling back to %s", h.configState.FallbackLogLevel)
			var resp *http.Response
			var req *http.Request
			req, err = h.buildLogLevelRequest(h.configState.FallbackLogLevel)
			if err != nil {
				return
			}
			resp, err = h.configHTTPClient.Do(req)
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

func (h *RemoteConfigHandler) buildLogLevelRequest(newLevel string) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf(h.configSetEndpointFormatString, newLevel), nil)
	if err != nil {
		pkglog.Infof("Failed to build request to change log level of the trace-agent to %s through remote config", newLevel)
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+h.agentConfig.AuthToken) // TODO IPC: avoid using the auth token directly
	return req, nil
}

func (h *RemoteConfigHandler) onUpdate(update map[string]state.RawConfig, _ func(string, state.ApplyStatus)) {
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
