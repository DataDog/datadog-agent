// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package apikeybridge keeps the Datadog OTel extension's API key in sync with the
// Agent's live configuration.
//
// The upstream `datadog` extension builds its own Agent config component and its own
// forwarder from the collector configuration, once, at construction. That forwarder
// already registers the domain resolver's rotation callback
// (comp/forwarder/defaultforwarder/impl.NewOptionsWithResolvers), but it registers it
// on the extension's private config, which nothing ever writes to again. The hook is
// armed and has no trigger, so a rotated API key never reaches the extension and it
// keeps 403ing on /api/v1/metadata until the process restarts.
//
// This package is the missing writer. It is supplied to the extension as an
// agentcomponents.ConfigOption and pushes the Agent's key into that private config
// whenever the Agent's own config changes.
package apikeybridge

import (
	"sync"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/agentcomponents"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

const apiKeySetting = "api_key"

// Option returns an agentcomponents.ConfigOption that mirrors `api_key` from the
// Agent's shared configuration into the Datadog extension's private one.
//
// It deliberately subscribes to the config rather than to any particular writer, so it
// covers every way the Agent's key can change: configsync pulling from a core Agent in
// connected mode (SourceLocalConfigProcess), and the otel-agent's own secrets backend
// refreshing an ENC[] handle in standalone mode (SourceSecret).
func Option(shared coreconfig.Component, logger log.Component) agentcomponents.ConfigOption {
	return func(priv pkgconfigmodel.Config) {
		// `shared` is fully constructed and can be read. `priv` cannot: this option runs
		// inside agentcomponents.NewConfigComponent, before BuildSchema, where a read
		// fails with "attempt to read key before config is constructed". Anything that
		// needs to read `priv` is therefore deferred to the callback below.
		armedKey := shared.GetString(apiKeySetting)
		if armedKey == "" {
			// No Agent key to track. The converter does not autoconfigure the extension
			// in this case either, so there is nothing to keep in sync.
			return
		}

		var (
			once     sync.Once
			tracking bool
		)

		shared.OnUpdate(func(setting string, _ pkgconfigmodel.Source, _, _ any, _ uint64, _ pkgconfigmodel.Source) {
			if setting != apiKeySetting {
				return
			}

			// Decide once, on the first rotation, whether this extension instance is
			// following the Agent's key at all. A user-declared `datadog/<name>`
			// extension with its own explicit key must be left alone — the converter's
			// reuseExtension does not touch it either. Comparing keys rather than
			// matching on the dd-autoconfigured instance name also covers a user who
			// declared the extension with the same key.
			once.Do(func() {
				tracking = priv.GetString(apiKeySetting) == armedKey
				if !tracking && logger != nil {
					logger.Debug("Datadog extension is configured with an API key that differs " +
						"from the Agent's; leaving it untouched on rotation")
				}
			})
			if !tracking {
				return
			}

			// Read the effective value rather than the callback's newValue: the callback
			// reports the value for the source that changed, and a higher-ranked source
			// (SourceRC, SourceCLI) may be masking it. We want the key the rest of the
			// Agent is actually using.
			key := shared.GetString(apiKeySetting)
			if key == "" {
				return
			}

			// SourceAgentRuntime matches the rank agentcomponents.WithAPIConfig seeded
			// the key at. A lower-ranked write would be silently discarded.
			priv.Set(apiKeySetting, key, pkgconfigmodel.SourceAgentRuntime)
		})
	}
}
