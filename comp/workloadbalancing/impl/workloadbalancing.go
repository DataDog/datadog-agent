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

	// appliedConfigs holds the last assignment applied for each Remote Config path, so an update
	// we cannot parse can fall back to it. Guarded by mu.
	appliedConfigs map[string]workloadBalancingRCConfig
}

func newWorkloadBalancingImpl(log log.Component, hostname hostnameinterface.Component, configs *workloadBalancingConfigs) *workloadBalancingImpl {
	return &workloadBalancingImpl{
		log:      log,
		hostname: hostname,
		configs:  configs,
		groups:   make(map[string]workloadbalancing.State),

		appliedConfigs: make(map[string]workloadBalancingRCConfig),
	}
}

func (w *workloadBalancingImpl) Enabled() bool {
	return w.configs.enabled
}

func (w *workloadBalancingImpl) GetGroupState(groupID string) workloadbalancing.State {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.stateLocked(groupID)
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

// setGroupLeaders replaces every tracked group at once. A group we currently hold that is absent
// from leaders is dropped rather than left behind, so an assignment that goes away returns its
// group to unmanaged and the group keeps reporting.
func (w *workloadBalancingImpl) setGroupLeaders(leaders map[string]string) error {
	agentHostname, err := w.hostname.Get(context.TODO())
	if err != nil {
		w.log.Warnf("error getting the hostname, leaving group state unchanged: %v", err)
		return err
	}

	groups := make(map[string]workloadbalancing.State, len(leaders))
	for groupID, leaderAgentHostname := range leaders {
		groups[groupID] = stateForLeader(agentHostname, leaderAgentHostname)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	for groupID, newState := range groups {
		w.logTransition(groupID, w.stateLocked(groupID), newState, agentHostname, leaders[groupID])
	}
	for groupID, prevState := range w.groups {
		if _, ok := groups[groupID]; !ok {
			w.logTransition(groupID, prevState, workloadbalancing.Unmanaged, agentHostname, "")
		}
	}
	w.groups = groups
	return nil
}

// stateForLeader maps the assigned Agent for a group onto the state this Agent holds for it. A
// group nobody has been assigned is unmanaged rather than standby: standby everywhere would leave
// the device unpolled, which is the one outcome workload balancing exists to avoid.
func stateForLeader(agentHostname string, leaderAgentHostname string) workloadbalancing.State {
	switch leaderAgentHostname {
	case "":
		return workloadbalancing.Unmanaged
	case agentHostname:
		return workloadbalancing.Active
	default:
		return workloadbalancing.Standby
	}
}

func (w *workloadBalancingImpl) stateLocked(groupID string) workloadbalancing.State {
	state, ok := w.groups[groupID]
	if !ok {
		return workloadbalancing.Unmanaged
	}
	return state
}

// logTransition records a state change, naming both hostnames on a standby. Matching is exact, so
// an assignment that names this host in a different form than hostname resolution returns reads as
// a legitimate standby, and the two names side by side are the only way to tell the difference.
func (w *workloadBalancingImpl) logTransition(groupID string, prevState workloadbalancing.State, newState workloadbalancing.State, agentHostname string, leaderAgentHostname string) {
	if newState == prevState {
		w.log.Debugf("group %s state not changed (current state: %s)", groupID, prevState)
		return
	}

	if newState == workloadbalancing.Standby {
		w.log.Infof("group %s state switched from %s to %s: assigned to %q, this Agent is %q",
			groupID, prevState, newState, leaderAgentHostname, agentHostname)
		return
	}
	w.log.Infof("group %s state switched from %s to %s", groupID, prevState, newState)
}

// previousAssignments returns the assignments applied by the last update.
func (w *workloadBalancingImpl) previousAssignments() map[string]workloadBalancingRCConfig {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return maps.Clone(w.appliedConfigs)
}

// storeAssignments records the assignments this update applied, for the next update to fall back on.
func (w *workloadBalancingImpl) storeAssignments(applied map[string]workloadBalancingRCConfig) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.appliedConfigs = applied
}
