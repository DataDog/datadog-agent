// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agenttelemetryimpl

import (
	"go.uber.org/atomic"

	pkgremoteflags "github.com/DataDog/datadog-agent/pkg/remoteflags"
)

const (
	// flagErrortracking turns error log forwarding on at runtime.
	flagErrortracking = "agent_errortracking"

	// errortrackingEnabledKey is the local setting. An explicit false there
	// always wins over the flag.
	errortrackingEnabledKey = "agent_telemetry.errortracking.enabled"
)

// remoteFlagSubscriber registers several flag handlers as one subscriber.
type remoteFlagSubscriber []pkgremoteflags.FlagHandler

// Handlers implements pkgremoteflags.RemoteFlagSubscriber.
func (s remoteFlagSubscriber) Handlers() []pkgremoteflags.FlagHandler { return s }

// errortrackingFlagHandler subscribes agent telemetry to the errortracking
// flag. It switches errortracking at runtime and turns off only what the flag
// itself turned on, so a local opt-in is never undone by the flag.
type errortrackingFlagHandler struct {
	// set switches errortracking and reports whether the state changed.
	set func(on bool) bool
	// localOff reports whether the local setting explicitly disables the feature.
	localOff func() bool
	healthy  func() bool

	// byFlag is true while errortracking is on because of the flag.
	byFlag atomic.Bool
}

func newErrortrackingFlagHandler(set func(on bool) bool, localOff, healthy func() bool) *errortrackingFlagHandler {
	return &errortrackingFlagHandler{set: set, localOff: localOff, healthy: healthy}
}

// FlagName returns the flag name.
func (h *errortrackingFlagHandler) FlagName() pkgremoteflags.FlagName {
	return flagErrortracking
}

// OnChange is called when the remote flag value changed.
func (h *errortrackingFlagHandler) OnChange(value pkgremoteflags.FlagValue) error {
	if !value {
		h.release()
		return nil
	}
	if h.localOff() {
		return nil
	}
	if h.set(true) {
		h.byFlag.Store(true)
	}
	return nil
}

// OnNoConfig is called when no config is present on the datacenter for current
// running agent.
func (h *errortrackingFlagHandler) OnNoConfig() {
	h.release()
}

// SafeRecover is called when something has gone wrong and we have to recover
// to a healthy state -> disable the feature.
func (h *errortrackingFlagHandler) SafeRecover(_ error, _ pkgremoteflags.FlagValue) {
	h.release()
}

// IsHealthy reports whether errortracking flushes are still succeeding. It is
// independent from the health of the other flag of the component.
func (h *errortrackingFlagHandler) IsHealthy() bool {
	return h.healthy()
}

// release switches errortracking off if the flag turned it on.
func (h *errortrackingFlagHandler) release() {
	if h.byFlag.Swap(false) {
		h.set(false)
	}
}

// errortrackingLocallyDisabled reports whether the local configuration sets
// agent_telemetry.errortracking.enabled to false explicitly. A disabled
// component has no configuration and reports true: nothing can be switched on.
func (a *atel) errortrackingLocallyDisabled() bool {
	if !a.enabled {
		return true
	}
	return a.cfgComp.IsConfigured(errortrackingEnabledKey) && !a.cfgComp.GetBool(errortrackingEnabledKey)
}
