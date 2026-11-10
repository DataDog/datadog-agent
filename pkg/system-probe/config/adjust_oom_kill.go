// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import "github.com/DataDog/datadog-agent/pkg/config/model"

// oomKillNS adds `oom_kill` namespace to configuration key
func oomKillNS(k ...string) string {
	return NSkey("oom_kill", k...)
}

// adjustOOMKill makes `oom_kill.enabled` and `system_probe_config.enable_oom_kill` equivalent:
// either one enables the OOM Kill probe, and `system_probe_config.enable_oom_kill` wins when both are set.
// Both keys are left holding the effective value, so readers of either one agree.
func adjustOOMKill(cfg model.Config) {
	newKey, oldKey := oomKillNS("enabled"), spNS("enable_oom_kill")

	var enabled bool
	switch {
	case cfg.IsConfigured(oldKey):
		enabled = cfg.GetBool(oldKey)
	case cfg.IsConfigured(newKey):
		enabled = cfg.GetBool(newKey)
	default:
		return
	}
	cfg.Set(newKey, enabled, model.SourceAgentRuntime)
	cfg.Set(oldKey, enabled, model.SourceAgentRuntime)
}
