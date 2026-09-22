// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package configstreamconsumerimpl

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/configstreambootstrap"
)

// configTarget names the config object an override is written to.
type configTarget int

const (
	// streamedTarget is the config object the snapshot is applied to.
	streamedTarget configTarget = iota
	// systemProbeTarget is the separate system-probe config object, which system-probe's logger
	// reads and the stream never writes to.
	systemProbeTarget
)

type override struct {
	// namespacedKey holds the per-agent value; when empty the agent uses baseKey's streamed value.
	namespacedKey string
	baseKey       string
	target        configTarget
}

// Per each agent binary, define which configstream-sender keys map to which configstream-receiver key.
var overridesByClient = map[string][]override{
	"security-agent": {{namespacedKey: "security_agent.log_level", baseKey: "log_level"}},
	"process-agent":  {{namespacedKey: "process_config.log_level", baseKey: "log_level"}},
	"trace-agent":    {{namespacedKey: "apm_config.log_level", baseKey: "log_level"}},
	// system_probe, not system_probe_config: the latter is the system-probe schema's own section, and the two objects hold different values for it.
	"system-probe":   {{namespacedKey: "system_probe.log_level", baseKey: "log_level", target: systemProbeTarget}},
}

// applyOverrides folds this client's namespaced settings onto their base keys, retractably.
func (c *consumer) applyOverrides() {
	overrides := overridesByClient[c.params.ClientName]
	if len(overrides) == 0 {
		return
	}
	streamed := configstreambootstrap.Config()
	for _, o := range overrides {
		cfg := streamed
		if o.target == systemProbeTarget {
			cfg = configstreambootstrap.SystemProbeConfig()
		}
		// Non-string values are dropped: pkg/util/log/setup's log_level callback asserts to string unchecked.
		value, _ := streamed.Get(o.namespacedKey).(string)
		if value == "" && o.target != streamedTarget {
			// The base key only carries the streamed value on the streamed object, so a write to
			// another object has to carry it across itself rather than fall through to it.
			value, _ = streamed.Get(o.baseKey).(string)
		}
		if value != "" {
			// SourceAgentRuntime outranks file/env yet still loses to a streamed RC/CLI value; Set panics on SourceEnvVar.
			cfg.Set(o.baseKey, value, pkgconfigmodel.SourceAgentRuntime)
			if c.appliedOverrides == nil {
				c.appliedOverrides = make(map[string]struct{}, len(overrides))
			}
			c.appliedOverrides[o.baseKey] = struct{}{}
			continue
		}
		if _, written := c.appliedOverrides[o.baseKey]; written {
			cfg.UnsetForSource(o.baseKey, pkgconfigmodel.SourceAgentRuntime)
			delete(c.appliedOverrides, o.baseKey)
		}
	}
}
