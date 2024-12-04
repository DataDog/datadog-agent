// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package haagentimpl

import (
	"context"
	"encoding/json"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"go.uber.org/atomic"
)

type haAgentImpl struct {
	log            log.Component
	haAgentConfigs *haAgentConfigs
	isLeader       *atomic.Bool
}

func newHaAgentImpl(log log.Component, haAgentConfigs *haAgentConfigs) *haAgentImpl {
	return &haAgentImpl{
		log:            log,
		haAgentConfigs: haAgentConfigs,
		isLeader:       atomic.NewBool(false),
	}
}

func (h *haAgentImpl) Enabled() bool {
	return h.haAgentConfigs.enabled
}

func (h *haAgentImpl) GetGroup() string {
	return h.haAgentConfigs.group
}

func (h *haAgentImpl) IsLeader() bool {
	return h.isLeader.Load()
}

func (h *haAgentImpl) SetLeader(leaderAgentHostname string) {
	agentHostname, err := hostname.Get(context.TODO())
	if err != nil {
		h.log.Warnf("error getting the hostname: %v", err)
		return
	}
	newIsLeader := agentHostname == leaderAgentHostname
	prevIsLeader := h.isLeader.Load()
	if newIsLeader != prevIsLeader {
		h.log.Infof("agent role switched from %s to %s", leaderStateToRole(prevIsLeader), leaderStateToRole(newIsLeader))
		h.isLeader.Store(newIsLeader)
	}
}

// ShouldRunIntegration return true if the agent integrations should to run.
// When ha-agent is disabled, the agent behave as standalone agent (non HA) and will always run all integrations.
func (h *haAgentImpl) ShouldRunIntegration(integrationName string) bool {
	if h.Enabled() && validHaIntegrations[integrationName] {
		return h.isLeader.Load()
	}
	return true
}

func (h *haAgentImpl) onHaAgentUpdate(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	h.log.Debugf("Updates received: count=%d", len(updates))

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
		if haAgentMsg.Group != h.GetGroup() {
			h.log.Warnf("Skipping invalid HA_AGENT update %s: expected group %s, got %s",
				configPath, h.GetGroup(), haAgentMsg.Group)
			applyStateCallback(configPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: "group does not match",
			})
			continue
		}

		h.SetLeader(haAgentMsg.Leader)

		h.log.Debugf("Processed config %s: %v", configPath, haAgentMsg)

		applyStateCallback(configPath, state.ApplyStatus{
			State: state.ApplyStateAcknowledged,
		})
	}
}
