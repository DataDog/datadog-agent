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
//	  "use_metric_pipeline": true,
//	  "components": {
//	    "bocpd": {
//	      "enabled": true,
//	      "warmup_points": 60,
//	      "hazard": 0.08,
//	      "cp_threshold": 0.55
//	    }
//	  }
//	}
//
// use_metric_pipeline is optional and selects the kind-routed metric workflow
// instead of the legacy detectors.
//
// anomaly_cutoff is optional and controls post-reactivation anomaly suppression;
// omitted fields inherit the enabled default.
type TestbenchParamsFile struct {
	UseMetricPipeline bool                       `json:"use_metric_pipeline"`
	AnomalyCutoff     *bool                      `json:"anomaly_cutoff"`
	Components        map[string]json.RawMessage `json:"components"`
}

// LoadTestbenchParams reads a params JSON file and returns ComponentSettings
// suitable for passing to Config.ComponentSettings.
//
// For each component mentioned in the file:
//   - "enabled" (if present) overrides the catalog default enabled state.
//   - remaining fields are passed to the component's parseJSON function (if
//     registered), overlaying the testbench replay defaults.
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

	settings, err := observerimpl.ParseTestbenchSettingsFromJSON(file.Components)
	if err != nil {
		return observerimpl.ComponentSettings{}, err
	}
	settings.UseMetricPipeline = file.UseMetricPipeline
	settings.MetricPipelineAnomalyCutoff = file.AnomalyCutoff
	return settings, nil
}
