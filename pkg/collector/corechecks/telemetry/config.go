// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"fmt"

	yaml "go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

// internalTelemetryConfig controls whether the check reports the Agent's internal telemetry
// registry in addition to the default-registry metrics it always reports.
type internalTelemetryConfig struct {
	// Enabled emits the curated set of internal telemetry metrics, giving parity with the
	// go_expvar `agent_stats.yaml.example` instance without scraping anything over HTTP.
	Enabled bool `yaml:"enabled"`
	// Advanced emits every metric registered in the internal telemetry registry instead of
	// the curated set. It implies Enabled.
	Advanced bool `yaml:"advanced"`
}

// instanceConfig is the check's instance-level configuration.
type instanceConfig struct {
	InternalTelemetry internalTelemetryConfig `yaml:"internal_telemetry"`
}

// parseInstanceConfig parses the raw instance YAML. An empty instance is valid and yields the
// zero value, which leaves the check's historical behavior untouched.
func parseInstanceConfig(data integration.Data) (instanceConfig, error) {
	var cfg instanceConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return instanceConfig{}, fmt.Errorf("invalid %s check configuration: %w", CheckName, err)
	}

	// Collecting everything only makes sense if we are collecting at all, so normalize here and
	// let the rest of the check test a single flag.
	if cfg.InternalTelemetry.Advanced {
		cfg.InternalTelemetry.Enabled = true
	}

	return cfg, nil
}
