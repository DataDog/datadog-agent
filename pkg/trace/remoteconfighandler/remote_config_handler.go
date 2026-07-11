// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package remoteconfighandler holds the logic responsible for updating the samplers when the remote configuration changes.
package remoteconfighandler

//go:generate go run go.uber.org/mock/mockgen -source=$GOFILE -package=$GOPACKAGE -destination=mock_samplers.go -build_constraint test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state/products/apmsampling"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/semantics"
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

// New creates a new RemoteConfigHandler.
func New(conf *config.AgentConfig, prioritySampler prioritySampler, rareSampler rareSampler, errorsSampler errorsSampler) *RemoteConfigHandler {
	if conf.RemoteConfigClient == nil {
		return nil
	}

	// The debug server is only required by AGENT_CONFIG (log-level writes).
	// If it's disabled and no other RC product is enabled, the handler has
	// nothing useful to do — disable RC. Otherwise we continue and skip just
	// the AGENT_CONFIG subscription in Start().
	if conf.DebugServerPort == 0 && !conf.RemoteConfigAPMSamplingEnabled && !conf.RemoteConfigAPMSemanticsEnabled {
		log.Errorf("debug server(apm_config.debug.port) was disabled and no other RC product is enabled, RC is disabled.")
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
		configSetEndpointFormatString: "https://127.0.0.1:" + strconv.Itoa(conf.DebugServerPort) + "/config/set?log_level=%s",
	}
}

// Start starts the remote config handler
func (h *RemoteConfigHandler) Start() {
	if h == nil {
		return
	}

	h.client.Start()
	if h.agentConfig.RemoteConfigAPMSamplingEnabled {
		h.client.Subscribe(state.ProductAPMSampling, h.onUpdate)
	}
	if h.agentConfig.RemoteConfigAgentConfigEnabled {
		// AGENT_CONFIG writes (log level overrides) require the debug server.
		// Skip the subscription if it isn't running so the rest of the handler
		// still works — see New() for the construction-time policy.
		if h.agentConfig.DebugServerPort == 0 {
			log.Warnf("AGENT_CONFIG remote config requested but the debug server (apm_config.debug.port) is disabled; skipping subscription.")
		} else {
			h.client.Subscribe(state.ProductAgentConfig, h.onAgentConfigUpdate)
		}
	}
	if h.agentConfig.RemoteConfigAPMSemanticsEnabled {
		h.client.Subscribe(state.ProductAPMSemanticCoreDD, h.onSemanticCoreUpdate)
	}
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

// onSemanticCoreUpdate handles APM_SEMANTIC_CORE_DD configurations.
//
// v1 contract: the publisher pushes a single full-registry payload (no partial
// updates). The RC protocol allows >1 config per product, so we defensively
// handle that case by warning and picking the lex-last valid cfgPath. We do
// not merge configs; that is intentional — RC payloads are full registries.
//
// Empty `concepts` payloads are rejected at parse time by NewRegistryFromJSON
// (treated as a pipeline bug, not an intentional wipe). When that happens, the
// previous registry stays live.
func (h *RemoteConfigHandler) onSemanticCoreUpdate(
	updates map[string]state.RawConfig,
	applyStateCallback func(string, state.ApplyStatus),
) {
	if len(updates) == 0 {
		// RC has nothing for us under this product: either the publisher has
		// not pushed anything yet, or a previously-applied config was deleted
		// or unassigned. In either case, revert to the embedded mappings so a
		// backend rollback/untargeting is reflected immediately rather than
		// leaving the agent stuck on the last RC payload until restart.
		embedded, err := semantics.NewEmbeddedRegistry()
		if err != nil {
			// Should not happen — the embedded mappings.json is validated at
			// process start. Log and leave the live registry in place.
			pkglog.Errorf("semantic-core RC: failed to load embedded registry while reverting: %v", err)
			return
		}
		if !semantics.RegistryEqual(embedded, semantics.DefaultRegistry()) {
			semantics.UpdateRegistry(embedded)
			pkglog.Infof("semantic-core RC: empty payload received; reverted to embedded registry content_hash=%s", embedded.ContentHash())
		}
		return
	}

	cfgPaths := make([]string, 0, len(updates))
	for p := range updates {
		cfgPaths = append(cfgPaths, p)
	}
	sort.Strings(cfgPaths)

	var chosen semantics.Registry
	var chosenPath string
	statuses := make(map[string]state.ApplyStatus, len(updates))
	for _, cfgPath := range cfgPaths {
		r, err := semantics.NewRegistryFromJSON(updates[cfgPath].Config)
		if err != nil {
			pkglog.Errorf("semantic-core RC update failed for %s: %v", cfgPath, err)
			statuses[cfgPath] = state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()}
			continue
		}
		statuses[cfgPath] = state.ApplyStatus{State: state.ApplyStateAcknowledged}
		// Last valid wins (cfgPaths is lex-sorted).
		chosen = r
		chosenPath = cfgPath
	}

	if len(cfgPaths) > 1 {
		pkglog.Warnf("semantic-core RC update delivered %d configs; expected 1. Using lex-last valid cfgPath: %s", len(cfgPaths), chosenPath)
	}

	if chosen != nil {
		if semantics.RegistryEqual(chosen, semantics.DefaultRegistry()) {
			// Same registry content_hash as the one already live: skip the swap.
			// Downstream consumers detect registry replacement themselves by
			// comparing the live registry's content_hash against their own cached
			// state, so we don't need to notify them explicitly.
			pkglog.Debugf("semantic-core RC payload %s matches the live registry content_hash; no-op", chosenPath)
		} else {
			semantics.UpdateRegistry(chosen)
			pkglog.Infof("semantic-core registry updated via RC: content_hash=%s, cfgPath=%s", chosen.ContentHash(), chosenPath)
		}
	}

	// Emit per-cfgPath statuses in deterministic order so callers and tests
	// see a stable sequence.
	for _, cfgPath := range cfgPaths {
		applyStateCallback(cfgPath, statuses[cfgPath])
	}
}
