// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agenttelemetryimpl

import (
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	pkgremoteflags "github.com/DataDog/datadog-agent/pkg/remoteflags"
)

const (
	// flagErrortracking turns error log forwarding on through the
	// configuration key below.
	flagErrortracking = "agent_errortracking"

	// errortrackingEnabledKey is the only thing the feature knows: the flag
	// drives it under the Remote Config source and the feature follows it.
	errortrackingEnabledKey = "agent_telemetry.errortracking.enabled"
)

// remoteFlagSubscriber registers several flag handlers as one subscriber.
type remoteFlagSubscriber []pkgremoteflags.FlagHandler

// Handlers implements pkgremoteflags.RemoteFlagSubscriber.
func (s remoteFlagSubscriber) Handlers() []pkgremoteflags.FlagHandler { return s }

// errortrackingFlagHandler binds the errortracking flag to its configuration
// key and reports the feature's health.
//
// It carries no decision of its own. The Remote Flags client writes the flag
// into agent_telemetry.errortracking.enabled under the Remote Config source,
// unless a local file, environment or other explicit setting already provides
// the key, and gives the key back when the flag is removed or when the feature
// is recovered. The feature reacts to the key itself (see onConfigUpdate), so
// a flag change, a removal and a recovery all end up in the same place.
//
// One limit comes from the configuration: the Remote Config source outranks
// fleet policies, so a fleet policy that sets the key after the flag was
// applied only takes effect once the flag gives the key back. Removing or
// disabling the flag is the way out.
type errortrackingFlagHandler struct {
	log     log.Component
	healthy func() bool
}

func newErrortrackingFlagHandler(logComp log.Component, healthy func() bool) *errortrackingFlagHandler {
	return &errortrackingFlagHandler{log: logComp, healthy: healthy}
}

// FlagName returns the flag name.
func (h *errortrackingFlagHandler) FlagName() pkgremoteflags.FlagName {
	return flagErrortracking
}

// ConfigurationField binds the flag to the key the feature reacts to.
func (h *errortrackingFlagHandler) ConfigurationField() string {
	return errortrackingEnabledKey
}

// OnChange is called when the flag value changed. The client has already
// written the key; the feature follows it.
func (h *errortrackingFlagHandler) OnChange(value pkgremoteflags.FlagValue) error {
	h.log.Debugf("errortracking remote flag is %v", bool(value))
	return nil
}

// OnNoConfig is called when the flag is absent from the received
// configuration. The client gives the key back.
func (h *errortrackingFlagHandler) OnNoConfig() {}

// SafeRecover is called when the flag failed to apply or the feature became
// unhealthy. The client has already given the key back, which switches the
// feature off.
func (h *errortrackingFlagHandler) SafeRecover(err error, _ pkgremoteflags.FlagValue) {
	h.log.Debugf("errortracking remote flag recovered: %v", err)
}

// IsHealthy reports whether errortracking flushes are still succeeding and the
// feature can run at all. It is independent from the health of the other
// flags of the component.
func (h *errortrackingFlagHandler) IsHealthy() bool {
	return h.healthy()
}

// onConfigUpdate follows agent_telemetry.errortracking.enabled. The key is
// read again under errortrackingSyncMu so that concurrent updates settle on
// the latest value.
func (a *atel) onConfigUpdate(key string, _ pkgconfigmodel.Source, _, _ any, _ uint64, _ pkgconfigmodel.Source) {
	if key != errortrackingEnabledKey {
		return
	}
	a.syncErrortracking()
}

// syncErrortracking switches errortracking to what the configuration says.
func (a *atel) syncErrortracking() {
	a.errortrackingSyncMu.Lock()
	defer a.errortrackingSyncMu.Unlock()
	a.setErrortracking(utils.IsErrorTrackingEnabled(a.cfgComp))
}
