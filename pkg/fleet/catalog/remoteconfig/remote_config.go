// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package remoteconfig adapts Remote Config updates to Fleet package catalogs.
// Product selection and catalog storage remain the responsibility of callers.
package remoteconfig

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/fleet/catalog"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ApplyFunc replaces a consumer's current catalog with a complete, validated
// catalog snapshot.
type ApplyFunc func(catalog.Catalog) error

// UpdateHandler is compatible with Remote Config subscription callbacks.
type UpdateHandler func(
	updates map[string]state.RawConfig,
	applyStatus func(string, state.ApplyStatus),
)

// NewUpdateHandler creates a callback that parses, validates, and merges a
// complete Remote Config catalog snapshot before passing it to apply. It does
// not subscribe to a product or retain the resulting catalog.
func NewUpdateHandler(apply ApplyFunc) UpdateHandler {
	return func(updates map[string]state.RawConfig, applyStatus func(string, state.ApplyStatus)) {
		catalogs := make([]catalog.Catalog, 0, len(updates))
		for configPath, update := range updates {
			parsedCatalog, err := catalog.Parse(update.Config)
			if err != nil {
				log.Errorf("could not unmarshal installer catalog: %s", err)
				applyStatus(configPath, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
				return
			}
			if err := parsedCatalog.Validate(); err != nil {
				log.Errorf("invalid package in catalog: %s", err)
				applyStatus(configPath, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
				return
			}
			catalogs = append(catalogs, parsedCatalog)
		}

		mergedCatalog := catalog.Merge(catalogs...)
		if apply == nil {
			reportApplyError(updates, applyStatus, errors.New("catalog apply function is not configured"))
			return
		}
		if err := apply(mergedCatalog); err != nil {
			log.Errorf("could not update catalog: %s", err)
			reportApplyError(updates, applyStatus, err)
			return
		}
		for configPath := range updates {
			applyStatus(configPath, state.ApplyStatus{State: state.ApplyStateAcknowledged})
		}
	}
}

func reportApplyError(
	updates map[string]state.RawConfig,
	applyStatus func(string, state.ApplyStatus),
	err error,
) {
	for configPath := range updates {
		applyStatus(configPath, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
	}
}
