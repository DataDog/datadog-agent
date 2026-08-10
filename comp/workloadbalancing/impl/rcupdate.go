// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package workloadbalancingimpl

import (
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// onWorkloadBalancingUpdate applies a full set of group assignments. Remote Config sends every
// config that targets this Agent on every update, so the set we receive is authoritative and any
// group missing from it no longer has an assignment.
//
// An empty update therefore clears every group, which is the opposite of what the HA Agent does
// with one. HA stops running its checks when it loses its assignment; we keep running ours,
// because the failure we care about is a device nobody is polling.
func (w *workloadBalancingImpl) onWorkloadBalancingUpdate(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	w.log.Debugf("Updates received: count=%d", len(updates))

	leaders := make(map[string]string, len(updates))
	valid := make([]string, 0, len(updates))
	for configPath, rawConfig := range updates {
		w.log.Debugf("Received config %s: %s", configPath, string(rawConfig.Config))

		var msg workloadBalancingRCConfig
		if err := json.Unmarshal(rawConfig.Config, &msg); err != nil {
			w.log.Warnf("Skipping invalid NDM_AGENT_WORKLOAD_BALANCING update %s: %v", configPath, err)
			applyStateCallback(configPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: "error unmarshalling payload",
			})
			continue
		}
		if msg.GroupID == "" {
			w.log.Warnf("Skipping invalid NDM_AGENT_WORKLOAD_BALANCING update %s: empty group_id", configPath)
			applyStateCallback(configPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: "group_id is empty",
			})
			continue
		}

		leaders[msg.GroupID] = msg.ActiveAgent
		valid = append(valid, configPath)
		w.log.Debugf("Processed config %s: %v", configPath, msg)
	}

	if err := w.setGroupLeaders(leaders); err != nil {
		for _, configPath := range valid {
			applyStateCallback(configPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
		}
		return
	}

	for _, configPath := range valid {
		applyStateCallback(configPath, state.ApplyStatus{
			State: state.ApplyStateAcknowledged,
		})
	}
}
