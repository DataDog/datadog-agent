// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttelemetryimpl

import (
	pkgremoteflags "github.com/DataDog/datadog-agent/pkg/remoteflags"

	"go.uber.org/atomic"
)

const (
	// flagTroubleshooting gates the "troubleshooting" profile; off unless Remote Config says otherwise.
	flagTroubleshooting = "troubleshooting_coat_bundle"

	// maxConsecutiveFlushFailures is how many back-to-back flushSession failures make us unhealthy.
	maxConsecutiveFlushFailures = 3
)

// remoteFlagHandler subscribes agent telemetry to one remote flag. The whole
// state of a gated profile is the atomic.Bool: nothing is started or stopped
// when the flag flips, the next scheduled tick simply reads it.
type remoteFlagHandler struct {
	name    pkgremoteflags.FlagName
	enabled atomic.Bool
	healthy func() bool
}

func newRemoteFlagHandler(name string, healthy func() bool) *remoteFlagHandler {
	return &remoteFlagHandler{name: pkgremoteflags.FlagName(name), healthy: healthy}
}

// Handlers implements pkgremoteflags.RemoteFlagSubscriber.
func (h *remoteFlagHandler) Handlers() []pkgremoteflags.FlagHandler {
	return []pkgremoteflags.FlagHandler{h}
}

// FlagName returns the flag name.
func (h *remoteFlagHandler) FlagName() pkgremoteflags.FlagName { return h.name }

// OnChange is called when the remote flag value changed.
func (h *remoteFlagHandler) OnChange(value pkgremoteflags.FlagValue) error {
	h.enabled.Store(bool(value))
	return nil
}

// OnNoConfig is called when no config is present on the datacenter for current
// running agent.
func (h *remoteFlagHandler) OnNoConfig() {
	h.enabled.Store(false)
}

// SafeRecover is called when something has gone wrong and we have to recover
// to a healthy state -> disable the feature.
func (h *remoteFlagHandler) SafeRecover(_ error, _ pkgremoteflags.FlagValue) {
	h.enabled.Store(false)
}

// IsHealthy reports whether the component is still healthy.
func (h *remoteFlagHandler) IsHealthy() bool {
	return h.healthy()
}

// enabledFor reports whether the flag named by a profile is on. A nil handler
// or any other name reports off, which is the safe state.
func (h *remoteFlagHandler) enabledFor(name string) bool {
	if h == nil || string(h.name) != name {
		return false
	}
	return h.enabled.Load()
}
