// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"fmt"
	"os"
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
//	    },
//	    "time_cluster": {
//	      "enabled": true,
//	      "proximity_seconds": 15,
//	      "min_cluster_size": 2
//	    },
//	    "log_pattern_extractor": {
//	      "min_cluster_size_before_emit": 5,
//	      "max_tokenized_string_length": 8000,
//	      "min_token_match_ratio": 0.5
//	    },
//	    "rrcf": { "enabled": false }
//	  }
//	}
//
// Components not listed in the file use their catalog defaults (both enabled
// state and hyperparameters). Listed components only override the fields
// they specify — unset hyperparameter fields keep their default values.
type TestbenchParamsFile struct {
	Components map[string]json.RawMessage `json:"components"`
}

// LoadTestbenchParams reads a params JSON file and returns ComponentSettings
// suitable for passing to TestBenchConfig.ComponentSettings.
//
// For each component mentioned in the file:
//   - "enabled" (if present) overrides the catalog default enabled state.
//   - remaining fields are passed to the component's parseJSON function (if
//     registered), overlaying the catalog default config.
//
// Returns an error if the file cannot be read, is invalid JSON, or references
// an unknown component name.
func LoadTestbenchParams(path string) (ComponentSettings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ComponentSettings{}, fmt.Errorf("reading params file %s: %w", path, err)
	}

	var file TestbenchParamsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return ComponentSettings{}, fmt.Errorf("parsing params file: %w", err)
	}

	catalog := defaultCatalog()

	// Build name→entry index for O(1) lookup.
	entryByName := make(map[string]componentEntry, len(catalog.entries))
	for _, e := range catalog.entries {
		entryByName[e.name] = e
	}

	settings := ComponentSettings{
		Enabled: make(map[string]bool),
		configs: make(map[string]any),
	}

	for name, raw := range file.Components {
		entry, ok := entryByName[name]
		if !ok {
			return ComponentSettings{}, fmt.Errorf("unknown component %q in params file", name)
		}

		// Extract the optional "enabled" field.
		var wrapper struct {
			Enabled *bool `json:"enabled"`
		}
		if err := json.Unmarshal(raw, &wrapper); err != nil {
			return ComponentSettings{}, fmt.Errorf("parsing \"enabled\" for component %q: %w", name, err)
		}
		if wrapper.Enabled != nil {
			settings.Enabled[name] = *wrapper.Enabled
		}

		// Parse hyperparameters if the component supports it.
		if entry.parseJSON != nil && entry.defaultConfig != nil {
			cfg, err := entry.parseJSON(entry.defaultConfig, raw)
			if err != nil {
				return ComponentSettings{}, fmt.Errorf("parsing hyperparameters for component %q: %w", name, err)
			}
			settings.configs[name] = cfg
		}
	}

	return settings, nil
}
