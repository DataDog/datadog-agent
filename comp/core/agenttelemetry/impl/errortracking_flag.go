// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agenttelemetryimpl

import (
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	pkgremoteflags "github.com/DataDog/datadog-agent/pkg/remoteflags"
)

const (
	// flagErrortracking turns error log forwarding on at runtime.
	flagErrortracking = "agent_errortracking"

	// errortrackingEnabledKey is the local setting. The flag drives it through
	// the Remote Config source, so an explicit local false always wins.
	errortrackingEnabledKey = "agent_telemetry.errortracking.enabled"
)

// newErrortrackingFlag returns the handler that drives the local setting from
// the errortracking flag. The component follows the setting itself, see
// followErrortrackingKey.
func (a *atel) newErrortrackingFlag() pkgremoteflags.FlagHandler {
	return pkgremoteflags.NewConfigKeyHandler(flagErrortracking, a.cfgComp, errortrackingEnabledKey, a.isErrortrackingHealthy)
}

// followErrortrackingKey switches errortracking whenever the local setting
// changes, from any source. The callback must not block, and the switch only
// takes errortrackingMu briefly: the release of a queue runs in the background.
func (a *atel) followErrortrackingKey() {
	a.cfgComp.OnUpdate(func(setting string, _ model.Source, _, _ any, _ uint64, _ model.Source) {
		if setting == errortrackingEnabledKey {
			a.setErrortrackingFrom(a.errortrackingConfigured)
		}
	})
}

// errortrackingConfigured reports whether the configuration enables errortracking.
func (a *atel) errortrackingConfigured() bool {
	return utils.IsErrorTrackingEnabled(a.cfgComp)
}
