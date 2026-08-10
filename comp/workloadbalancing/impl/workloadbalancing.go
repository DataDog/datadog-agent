// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package workloadbalancingimpl

import (
	"context"
	"maps"
	"sync"

	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	workloadbalancing "github.com/DataDog/datadog-agent/comp/workloadbalancing/def"
)

type workloadBalancingImpl struct {
	log      log.Component
	hostname hostnameinterface.Component
	configs  *workloadBalancingConfigs

	mu     sync.RWMutex
	groups map[string]workloadbalancing.State
}

func newWorkloadBalancingImpl(log log.Component, hostname hostnameinterface.Component, configs *workloadBalancingConfigs) *workloadBalancingImpl {
	return &workloadBalancingImpl{
		log:      log,
		hostname: hostname,
		configs:  configs,
		groups:   make(map[string]workloadbalancing.State),
	}
}

func (w *workloadBalancingImpl) Enabled() bool {
	return w.configs.enabled
}

func (w *workloadBalancingImpl) GetGroupState(groupID string) workloadbalancing.State {
	w.mu.RLock()
	defer w.mu.RUnlock()

	state, ok := w.groups[groupID]
	if !ok {
		return workloadbalancing.Unmanaged
	}
	return state
}

func (w *workloadBalancingImpl) GetGroupStates() map[string]workloadbalancing.State {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return maps.Clone(w.groups)
}

// IsGroupActive suppresses a group only on an explicit standby. Anything else, including a
// group we have never been told about, runs. Duplicate monitoring during a handoff is
// recoverable, a silent device is not.
func (w *workloadBalancingImpl) IsGroupActive(groupID string) bool {
	return w.GetGroupState(groupID) != workloadbalancing.Standby
}

func (w *workloadBalancingImpl) SetGroupLeader(groupID string, leaderAgentHostname string) {
	agentHostname, err := w.hostname.Get(context.TODO())
	if err != nil {
		w.log.Warnf("error getting the hostname, leaving group %s unchanged: %v", groupID, err)
		return
	}

	newState := workloadbalancing.Standby
	if agentHostname == leaderAgentHostname {
		newState = workloadbalancing.Active
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	prevState, ok := w.groups[groupID]
	if !ok {
		prevState = workloadbalancing.Unmanaged
	}
	w.groups[groupID] = newState

	if newState != prevState {
		w.log.Infof("group %s state switched from %s to %s", groupID, prevState, newState)
	} else {
		w.log.Debugf("group %s state not changed (current state: %s)", groupID, prevState)
	}
}

func (w *workloadBalancingImpl) RemoveGroup(groupID string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if prevState, ok := w.groups[groupID]; ok {
		delete(w.groups, groupID)
		w.log.Infof("group %s removed, state was %s", groupID, prevState)
	}
}
