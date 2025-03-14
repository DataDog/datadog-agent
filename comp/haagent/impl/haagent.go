// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package haagentimpl

import (
	"context"
	"encoding/json"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	haagent "github.com/DataDog/datadog-agent/comp/haagent/def"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"go.uber.org/atomic"
)

type haAgentImpl struct {
	log            log.Component
	haAgentConfigs *haAgentConfigs
	state          *atomic.String
}

func newHaAgentImpl(log log.Component, haAgentConfigs *haAgentConfigs) *haAgentImpl {
	return &haAgentImpl{
		log:            log,
		haAgentConfigs: haAgentConfigs,
		state:          atomic.NewString(string(haagent.Unknown)),
	}
}

func (h *haAgentImpl) Enabled() bool {
	return h.haAgentConfigs.enabled
}

func (h *haAgentImpl) GetConfigID() string {
	return h.haAgentConfigs.configID
}

func (h *haAgentImpl) GetState() haagent.State {
	return haagent.State(h.state.Load())
}

func (h *haAgentImpl) SetLeader(leaderAgentHostname string) {
	agentHostname, err := hostname.Get(context.TODO())
	if err != nil {
		h.log.Warnf("error getting the hostname: %v", err)
		return
	}

	var newState haagent.State
	if agentHostname == leaderAgentHostname {
		newState = haagent.Active
	} else {
		newState = haagent.Standby
	}

	prevState := h.GetState()

	if newState != prevState {
		h.log.Infof("agent state switched from %s to %s", prevState, newState)
		h.state.Store(string(newState))
	} else {
		h.log.Debugf("agent state not changed (current state: %s)", prevState)
	}
}

func (h *haAgentImpl) resetAgentState() {
	h.state.Store(string(haagent.Unknown))
}

func (h *haAgentImpl) IsActive() bool {
	return h.GetState() == haagent.Active
}

func (h *haAgentImpl) onHaAgentUpdate(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	h.log.Debugf("Updates received: count=%d", len(updates))

	// New updates arrived, but if the list of updates is empty,
	// it means we don't have any updates applying to this agent anymore.
	// In this case, reset HA Agent setting to default states.
	if len(updates) == 0 {
		h.log.Warn("Empty update received. Resetting Agent State to Unknown.")
		h.resetAgentState()
		return
	}

	for configPath, rawConfig := range updates {
		h.log.Debugf("Received config %s: %s", configPath, string(rawConfig.Config))
		haAgentMsg := haAgentConfig{}
		err := json.Unmarshal(rawConfig.Config, &haAgentMsg)
		if err != nil {
			h.log.Warnf("Skipping invalid HA_AGENT update %s: %v", configPath, err)
			applyStateCallback(configPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: "error unmarshalling payload",
			})
			continue
		}
		if haAgentMsg.ConfigID != h.GetConfigID() {
			h.log.Warnf("Skipping invalid HA_AGENT update %s: expected configID %s, got %s",
				configPath, h.GetConfigID(), haAgentMsg.ConfigID)
			applyStateCallback(configPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: "config_id does not match",
			})
			continue
		}

		h.SetLeader(haAgentMsg.ActiveAgent)

		h.log.Debugf("Processed config %s: %v", configPath, haAgentMsg)

		applyStateCallback(configPath, state.ApplyStatus{
			State: state.ApplyStateAcknowledged,
		})
	}
}
