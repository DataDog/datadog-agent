// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

import (
	"encoding/json"
	"fmt"
	"os"

	observerimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/impl"
)

// TestbenchParamsFile is the top-level structure of a --config JSON file used
// for Bayesian optimization. Each component entry controls enabled state and
// hyperparameters for the named component.
//
// Example:
//
//	{
//	  "components": {
//	    "bocpd": {
//	      "enabled": true,
//	      "warmup_points": 60,
//	      "hazard": 0.08,
//	      "cp_threshold": 0.55
//	    }
//	  }
//	}
type TestbenchParamsFile struct {
	Components map[string]json.RawMessage `json:"components"`
}

// LoadTestbenchParams reads a params JSON file and returns ComponentSettings
// suitable for passing to Config.ComponentSettings.
//
// For each component mentioned in the file:
//   - "enabled" (if present) overrides the catalog default enabled state.
//   - remaining fields are passed to the component's parseJSON function (if
//     registered), overlaying the catalog default config.
//
// Returns an error if the file cannot be read, is invalid JSON, or references
// an unknown component name.
func LoadTestbenchParams(path string) (observerimpl.ComponentSettings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return observerimpl.ComponentSettings{}, fmt.Errorf("reading params file %s: %w", path, err)
	}

	var file TestbenchParamsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return observerimpl.ComponentSettings{}, fmt.Errorf("parsing params file: %w", err)
	}

	// Build name→entry index for O(1) lookup using the exported catalog.
	entries := observerimpl.TestbenchCatalogEntries()
	entryNames := make(map[string]bool, len(entries))
	for _, e := range entries {
		entryNames[e.Name] = true
	}

	settings := observerimpl.ComponentSettings{
		Enabled: make(map[string]bool),
	}

	for name, raw := range file.Components {
		if !entryNames[name] {
			return observerimpl.ComponentSettings{}, fmt.Errorf("unknown component %q in params file", name)
		}

		// Extract the optional "enabled" field.
		var wrapper struct {
			Enabled *bool `json:"enabled"`
		}
		if err := json.Unmarshal(raw, &wrapper); err != nil {
			return observerimpl.ComponentSettings{}, fmt.Errorf("parsing \"enabled\" for component %q: %w", name, err)
		}
		if wrapper.Enabled != nil {
			settings.Enabled[name] = *wrapper.Enabled
		}
	}

	return settings, nil
}
