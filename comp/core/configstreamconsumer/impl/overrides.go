// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package configstreamconsumerimpl

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/configstreambootstrap"
)

// Declared entry by entry: most namespaced keys sharing a trailing name mean something different.
// No system-probe entry: its key is absent from the core schema and config.Adjust already folds it on the sysprobe object.
var overridesByClient = map[string]map[string]string{
	"security-agent": {"security_agent.log_level": "log_level"},
	"process-agent":  {"process_config.log_level": "log_level"},
	"trace-agent":    {"apm_config.log_level": "log_level"},
}

// applyOverrides folds this client's namespaced settings onto their base keys, retractably.
func (c *consumer) applyOverrides() {
	overrides := overridesByClient[c.params.ClientName]
	if len(overrides) == 0 {
		return
	}
	cfg := configstreambootstrap.Config()
	for namespacedKey, baseKey := range overrides {
		// Non-string values are dropped: pkg/util/log/setup's log_level callback asserts to string unchecked.
		value, _ := cfg.Get(namespacedKey).(string)
		if value != "" {
			// SourceAgentRuntime outranks file/env yet still loses to a streamed RC/CLI value; Set panics on SourceEnvVar.
			cfg.Set(baseKey, value, pkgconfigmodel.SourceAgentRuntime)
			if c.appliedOverrides == nil {
				c.appliedOverrides = make(map[string]struct{}, len(overrides))
			}
			c.appliedOverrides[baseKey] = struct{}{}
			continue
		}
		if _, written := c.appliedOverrides[baseKey]; written {
			cfg.UnsetForSource(baseKey, pkgconfigmodel.SourceAgentRuntime)
			delete(c.appliedOverrides, baseKey)
		}
	}
}
