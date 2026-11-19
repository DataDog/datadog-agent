// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventoryagentimpl

import (
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgremoteflags "github.com/DataDog/datadog-agent/pkg/remoteflags"
)

const (
	// flagDropFullConfiguration is the remote flag letting Datadog drop full_configuration from the
	// inventory payload. Enabled means "drop".
	flagDropFullConfiguration = "inventory_drop_full_configuration"

	fullConfigurationSetting = "inventories_full_configuration_enabled"
)

// Tri-state of the remote flag.
const (
	flagUnknown int32 = iota
	flagDrop
	flagKeep
)

type settingReader interface {
	GetBool(key string) bool
	GetSource(key string) model.Source
}

// fullConfigFlagHandler subscribes the inventory payload to the remote flag dropping full_configuration.
type fullConfigFlagHandler struct {
	conf    settingReader
	refresh func()
	state   atomic.Int32
}

func newFullConfigFlagHandler(conf settingReader, refresh func()) *fullConfigFlagHandler {
	return &fullConfigFlagHandler{conf: conf, refresh: refresh}
}

// Handlers implements pkgremoteflags.RemoteFlagSubscriber.
func (h *fullConfigFlagHandler) Handlers() []pkgremoteflags.FlagHandler {
	return []pkgremoteflags.FlagHandler{h}
}

// FlagName returns the flag name.
func (h *fullConfigFlagHandler) FlagName() pkgremoteflags.FlagName {
	return pkgremoteflags.FlagName(flagDropFullConfiguration)
}

// OnChange is called when the remote flag value changed.
func (h *fullConfigFlagHandler) OnChange(value pkgremoteflags.FlagValue) error {
	if value {
		h.set(flagDrop)
	} else {
		h.set(flagKeep)
	}
	return nil
}

// OnNoConfig is called when the flag is absent from the remote configuration: back to the local setting.
func (h *fullConfigFlagHandler) OnNoConfig() { h.set(flagUnknown) }

// SafeRecover resets to the local setting.
func (h *fullConfigFlagHandler) SafeRecover(_ error, _ pkgremoteflags.FlagValue) { h.set(flagUnknown) }

// IsHealthy always reports healthy: the flag only toggles a payload field.
func (h *fullConfigFlagHandler) IsHealthy() bool { return true }

// set stores the new state and refreshes the payload if the effective value changed.
func (h *fullConfigFlagHandler) set(state int32) {
	before := h.fullConfigurationEnabled()
	h.state.Store(state)
	if before != h.fullConfigurationEnabled() && h.refresh != nil {
		h.refresh()
	}
}

// userSet reports whether the setting was explicitly set by something other than the built-in default.
func (h *fullConfigFlagHandler) userSet() bool {
	switch h.conf.GetSource(fullConfigurationSetting) {
	case model.SourceDefault, model.SourceSchema, model.SourceUnknown, model.SourceInfraMode, "":
		return false
	default:
		return true
	}
}

// fullConfigurationEnabled resolves whether full_configuration is included in the payload: an explicit
// user setting wins, then the remote flag, then the default of the setting.
func (h *fullConfigFlagHandler) fullConfigurationEnabled() bool {
	if h.userSet() {
		return h.conf.GetBool(fullConfigurationSetting)
	}
	switch h.state.Load() {
	case flagDrop:
		return false
	case flagKeep:
		return true
	}
	return h.conf.GetBool(fullConfigurationSetting)
}
