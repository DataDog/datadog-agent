// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventoryagentimpl

import (
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgremoteflags "github.com/DataDog/datadog-agent/pkg/remoteflags"
)

const (
	// flagDropFullConfiguration is the remote flag letting Datadog drop full_configuration from the
	// agent metadata payload. Enabled means "drop".
	flagDropFullConfiguration = "inventory_drop_full_configuration"

	fullConfigurationSetting = "inventories_full_configuration_enabled"
)

type settingWriter interface {
	GetSource(key string) model.Source
	Set(key string, value interface{}, source model.Source)
	UnsetForSource(key string, source model.Source)
}

// fullConfigFlagHandler applies the remote flag dropping full_configuration to the
// inventories_full_configuration_enabled setting, under the Remote Config source, so that the
// configuration reflects what the Agent sends.
type fullConfigFlagHandler struct {
	conf settingWriter
	log  log.Component
}

func newFullConfigFlagHandler(conf settingWriter, l log.Component) *fullConfigFlagHandler {
	return &fullConfigFlagHandler{conf: conf, log: l}
}

// Handlers implements pkgremoteflags.RemoteFlagSubscriber.
func (h *fullConfigFlagHandler) Handlers() []pkgremoteflags.FlagHandler {
	return []pkgremoteflags.FlagHandler{h}
}

// FlagName returns the flag name.
func (h *fullConfigFlagHandler) FlagName() pkgremoteflags.FlagName {
	return pkgremoteflags.FlagName(flagDropFullConfiguration)
}

// OnChange sets the setting from the flag, unless it was set explicitly: any source other than the
// default, or this handler's previous value, wins over the flag.
func (h *fullConfigFlagHandler) OnChange(drop pkgremoteflags.FlagValue) error {
	switch source := h.conf.GetSource(fullConfigurationSetting); source {
	case model.SourceDefault, model.SourceRC:
		h.conf.Set(fullConfigurationSetting, !bool(drop), model.SourceRC)
	default:
		h.log.Infof("Remote flag %s: not applied, %s is set by %s", flagDropFullConfiguration, fullConfigurationSetting, source)
	}
	return nil
}

// OnNoConfig is called when the flag is absent from the remote configuration: back to the local setting.
func (h *fullConfigFlagHandler) OnNoConfig() { h.unset() }

// SafeRecover goes back to the local setting.
func (h *fullConfigFlagHandler) SafeRecover(_ error, _ pkgremoteflags.FlagValue) { h.unset() }

// IsHealthy always reports healthy: the flag only toggles a payload field.
func (h *fullConfigFlagHandler) IsHealthy() bool { return true }

func (h *fullConfigFlagHandler) unset() {
	h.conf.UnsetForSource(fullConfigurationSetting, model.SourceRC)
}
