// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !aix

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
// covers every way the Agent's key can change: configsync pulling from a core Agent
// (SourceLocalConfigProcess), and a secrets backend refreshing an ENC[] handle
// (SourceSecret). Only connected mode matters in practice -- the converter does not
// autoconfigure this extension when otel_standalone is set.
func Option(shared coreconfig.Component, logger log.Component) agentcomponents.ConfigOption {
	return func(priv pkgconfigmodel.Config) {
		armedKey := shared.GetString(apiKeySetting)
		if armedKey == "" {
			// No Agent key to track. The converter does not autoconfigure the extension
			// in this case either, so there is nothing to keep in sync.
			return
		}

		// A user-declared `datadog/<name>` extension with its own explicit key must be
		// left alone -- the converter's reuseExtension does not touch it either.
		// Comparing keys rather than matching on the dd-autoconfigured instance name
		// also covers a user who declared the extension with the same key.
		//
		// The seeded key came from this same setting one confmap resolution earlier
		// (converter/impl/autoconfigure.go). A rotation landing in that window makes the
		// two differ and leaves the autoconfigured extension untracked until restart.
		// ConfigOption carries no instance ID, so the key is the only discriminator.
		if privateKey(priv) != armedKey {
			if logger != nil {
				logger.Debug("Datadog extension is configured with an API key that differs " +
					"from the Agent's; leaving it untouched on rotation")
			}
			return
		}

		// The read and the write are one critical section: notification receivers are
		// invoked outside the config lock, so two overlapping rotations could otherwise
		// be applied in the order new-then-old and leave the stale key behind.
		var mu sync.Mutex
		push := func() {
			mu.Lock()
			defer mu.Unlock()
			// Read the effective value rather than the callback's newValue: the callback
			// reports the value for the source that changed, and a higher-ranked source
			// (SourceRC, SourceCLI) may be masking it. We want the key the rest of the
			// Agent is actually using.
			key := shared.GetString(apiKeySetting)
			if key == "" {
				// An emptied key is not propagated: clearing the extension's would only
				// break it, so it keeps the last good one until a real key arrives.
				return
			}
			// SourceAgentRuntime matches the rank agentcomponents.WithAPIConfig seeded
			// the key at. A lower-ranked write would be silently discarded.
			priv.Set(apiKeySetting, key, pkgconfigmodel.SourceAgentRuntime)
		}

		shared.OnUpdate(func(setting string, _ pkgconfigmodel.Source, _, _ any, _ uint64, _ pkgconfigmodel.Source) {
			if setting != apiKeySetting {
				return
			}
			push()
		})

		// Reconciles a rotation that landed between reading armedKey and subscribing.
		// Writing to `priv` is safe this early: Set only touches its source layer, which
		// the BuildSchema still to come then merges.
		push()
	}
}

// privateKey returns the extension's own `api_key`, as seeded by
// agentcomponents.WithAPIConfig a few options earlier.
//
// A plain Get cannot be used here: this runs inside NewConfigComponent, before
// BuildSchema, where reads log "attempt to read key before config is constructed" and
// return the zero value. SetWithLock reads the source layer under the config's own write
// lock, and apply=false cancels the write before anything is stored or notified.
func privateKey(priv pkgconfigmodel.Config) string {
	var key string
	priv.SetWithLock(apiKeySetting, pkgconfigmodel.SourceAgentRuntime, func(oldValue any, _ pkgconfigmodel.Source) (any, bool) {
		key, _ = oldValue.(string)
		return nil, false
	})
	return key
}
