// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package workloadbalancingimpl

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	haagent "github.com/DataDog/datadog-agent/comp/haagent/def"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// groupState is the state this Agent holds for a workload balancing group. The zero value
// (unset, i.e. a group missing from the map) means unmanaged and is treated as active: this
// Agent runs the group's checks unless explicitly told to stand down.
type groupState string

const (
	groupStateActive  groupState = "active"
	groupStateStandby groupState = "standby"
)

type workloadBalancingImpl struct {
	log      log.Component
	hostname hostnameinterface.Component
	haAgent  haagent.Component
	configs  *workloadBalancingConfigs

	mu     sync.RWMutex
	groups map[string]groupState
}

func newWorkloadBalancingImpl(log log.Component, hostname hostnameinterface.Component, haAgent haagent.Component, configs *workloadBalancingConfigs) *workloadBalancingImpl {
	return &workloadBalancingImpl{
		log:      log,
		hostname: hostname,
		haAgent:  haAgent,
		configs:  configs,
		groups:   make(map[string]groupState),
	}
}

func (w *workloadBalancingImpl) IsGroupActive(groupID string) bool {
	if !w.configs.enabled {
		return true
	}

	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.groups[groupID] != groupStateStandby
}

func (w *workloadBalancingImpl) Enabled() bool {
	return w.configs.enabled
}

func stateForActiveAgent(agentHostname, activeAgent string) groupState {
	if agentHostname == activeAgent {
		return groupStateActive
	}
	return groupStateStandby
}

// onWorkloadBalancingUpdate handles workload-balancing documents on the shared HA_AGENT
// product. RC delivers the full document set each poll, not a delta: a group's assignment is
// only cleared when its document stops appearing, not because this poll happens to lack it.
func (w *workloadBalancingImpl) onWorkloadBalancingUpdate(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	agentHostname, err := w.hostname.Get(context.TODO())
	if err != nil {
		w.log.Warnf("error getting the hostname, skipping workload balancing update: %v", err)
		return
	}

	haAgentEnabled := w.haAgent.Enabled()

	w.mu.RLock()
	previousGroups := w.groups
	w.mu.RUnlock()

	seenGroups := make(map[string]groupState, len(previousGroups))

	for configPath, rawConfig := range updates {
		var discriminator haagent.WorkloadBalancingDiscriminator
		if err := json.Unmarshal(rawConfig.Config, &discriminator); err != nil || discriminator.Type != haagent.WorkloadBalancingType {
			if !haAgentEnabled {
				// comp/haagent isn't listening on this Agent to claim it either; report status
				// ourselves so nothing is silently dropped just because Failover mode is off.
				applyStateCallback(configPath, state.ApplyStatus{
					State: state.ApplyStateError,
					Error: "not a workload balancing document",
				})
			}
			continue
		}

		var cfg workloadBalancingRCConfig
		parseErr := json.Unmarshal(rawConfig.Config, &cfg)
		if parseErr != nil || cfg.WorkloadBalancingGroupID == "" || cfg.ActiveAgent == "" {
			var applyErr string
			switch {
			case parseErr != nil:
				applyErr = "error unmarshalling payload"
			case cfg.WorkloadBalancingGroupID == "":
				applyErr = "missing workload_balancing_group_id"
			default:
				// An empty active_agent isn't "nobody's active yet": stateForActiveAgent would
				// put every Agent on standby, turning an incomplete document into a silent
				// outage instead of the safe direction (duplicate over gap).
				applyErr = "missing active_agent"
			}
			w.log.Warnf("Skipping invalid workload balancing update %s: %s", configPath, applyErr)
			applyStateCallback(configPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: applyErr,
			})
			// keepPreviousAssignment: a malformed document isn't the same as an absent one. Only
			// possible when the group ID itself parsed despite another field failing, and only
			// if this pass hasn't already computed a real value for that group.
			if cfg.WorkloadBalancingGroupID != "" {
				if _, done := seenGroups[cfg.WorkloadBalancingGroupID]; !done {
					if prev, ok := previousGroups[cfg.WorkloadBalancingGroupID]; ok {
						seenGroups[cfg.WorkloadBalancingGroupID] = prev
					}
				}
			}
			continue
		}

		seenGroups[cfg.WorkloadBalancingGroupID] = stateForActiveAgent(agentHostname, cfg.ActiveAgent)
		applyStateCallback(configPath, state.ApplyStatus{
			State: state.ApplyStateAcknowledged,
		})
	}

	w.mu.Lock()
	w.groups = seenGroups
	w.mu.Unlock()
}
