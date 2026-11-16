// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agenttelemetryimpl

import (
	"sync"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgremoteflags "github.com/DataDog/datadog-agent/pkg/remoteflags"
)

const (
	// flagErrortracking turns error log forwarding on when the local
	// configuration does not say otherwise.
	flagErrortracking = "agent_errortracking"

	// errortrackingEnabledKey is the local switch the flag defers to.
	errortrackingEnabledKey = "agent_telemetry.errortracking.enabled"
)

// remoteFlagSubscriber registers several flag handlers as one subscriber.
type remoteFlagSubscriber []pkgremoteflags.FlagHandler

// Handlers implements pkgremoteflags.RemoteFlagSubscriber.
func (s remoteFlagSubscriber) Handlers() []pkgremoteflags.FlagHandler { return s }

// errortrackingFlagHandler drives errortracking from the remote flag.
//
// Precedence is enforced here, not by the Remote Flags client: the backend
// only sends the flag name and value, so there is no override to honor.
//
//	local setting | flag                          | result
//	true          | any                           | on
//	false         | any                           | off
//	unset         | true                          | on
//	unset         | false, absent or recovered    | off
//
// An absent flag and a recovery therefore only switch off what the flag
// switched on; a local true is never undone. A setting counts as local when
// it comes from any source other than the schema default.
//
// The Remote Flags client delivers a change from its own goroutine but an
// absent flag inline, so a value that was just removed can be followed by a
// late change that turns the feature on while the flag is absent. The next
// event corrects it.
type errortrackingFlagHandler struct {
	cfg     pkgconfigmodel.Reader
	log     log.Component
	apply   func(on bool)
	healthy func() bool

	// mu keeps the resulting apply calls in the order the flag values arrive
	// across the client's update and health monitor goroutines.
	mu sync.Mutex
}

func newErrortrackingFlagHandler(cfg pkgconfigmodel.Reader, logComp log.Component, apply func(on bool), healthy func() bool) *errortrackingFlagHandler {
	return &errortrackingFlagHandler{cfg: cfg, log: logComp, apply: apply, healthy: healthy}
}

// FlagName returns the flag name.
func (h *errortrackingFlagHandler) FlagName() pkgremoteflags.FlagName {
	return flagErrortracking
}

// OnChange is called when the remote flag value changed.
func (h *errortrackingFlagHandler) OnChange(value pkgremoteflags.FlagValue) error {
	h.set(bool(value))
	return nil
}

// OnNoConfig is called when the flag is absent from the received
// configuration.
func (h *errortrackingFlagHandler) OnNoConfig() {
	h.set(false)
}

// SafeRecover switches off what the flag switched on.
func (h *errortrackingFlagHandler) SafeRecover(_ error, _ pkgremoteflags.FlagValue) {
	h.set(false)
}

// IsHealthy reports whether errortracking flushes are still succeeding. It
// is independent from the health of the other flags of the component.
func (h *errortrackingFlagHandler) IsHealthy() bool {
	return h.healthy()
}

func (h *errortrackingFlagHandler) set(flagOn bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	on := flagOn
	local := h.cfg.IsConfigured(errortrackingEnabledKey)
	if local {
		on = h.cfg.GetBool(errortrackingEnabledKey)
	}
	h.log.Debugf("errortracking remote flag is %v, local setting present: %v, errortracking on: %v", flagOn, local, on)
	h.apply(on)
}
