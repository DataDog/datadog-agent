// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build otlp

package otlp

import (
	"fmt"
	"strings"

	"go.opentelemetry.io/collector/confmap"
	"go.uber.org/multierr"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/mohae/deepcopy"
)

func portToUint(v int) (port uint, err error) {
	if v < 0 || v > 65535 {
		err = fmt.Errorf("%d is out of [0, 65535] range", v)
	}
	port = uint(v)
	return
}

// readConfigSection from a config.Config object.
func readConfigSection(cfg config.Config, section string) *confmap.Conf {
	// Viper doesn't work well when getting subsections, since it
	// ignores environment variables and nil-but-present sections.
	// To work around this, we do the following two steps:

	// Step one works around https://github.com/spf13/viper/issues/819
	// If we only had the stuff below, the nil sections would be ignored.
	// We want to take into account nil-but-present sections.
	//
	// Furthermore, Viper returns an `interface{}` nil in the case where
	// `section` is present but empty: e.g. we want to read
	//	"otlp_config.receiver", but we have
	//
	//         otlp_config:
	//           receiver:
	//
	// `GetStringMap` it will fail to cast `interface{}` nil to
	// `map[string]interface{}` nil; we use `Get` and cast manually.
	rawVal := cfg.Get(section)
	stringMap := map[string]interface{}{}
	if val, ok := rawVal.(map[string]interface{}); ok {
		// deep copy since `cfg.Get` returns a reference
		stringMap = deepcopy.Copy(val).(map[string]interface{})
	}

	// Step two works around https://github.com/spf13/viper/issues/1012
	// we check every key manually, and if it belongs to the OTLP receiver section,
	// we set it. We need to do this to account for environment variable values.
	prefix := section + "."
	for _, key := range cfg.AllKeys() {
		if strings.HasPrefix(key, prefix) && cfg.IsSet(key) {
			mapKey := strings.ReplaceAll(key[len(prefix):], ".", confmap.KeyDelimiter)
			// deep copy since `cfg.Get` returns a reference
			stringMap[mapKey] = deepcopy.Copy(cfg.Get(key))
		}
	}
	return confmap.NewFromStringMap(stringMap)
}

// FromAgentConfig builds a pipeline configuration from an Agent configuration.
func FromAgentConfig(cfg config.Config) (PipelineConfig, error) {
	var errs []error
	otlpConfig := readConfigSection(cfg, config.OTLPReceiverSection)

	tracePort, err := portToUint(cfg.GetInt(config.OTLPTracePort))
	if err != nil {
		errs = append(errs, fmt.Errorf("internal trace port is invalid: %w", err))
	}

	metricsEnabled := cfg.GetBool(config.OTLPMetricsEnabled)
	tracesEnabled := cfg.GetBool(config.OTLPTracesEnabled)
	if !metricsEnabled && !tracesEnabled {
		errs = append(errs, fmt.Errorf("at least one OTLP signal needs to be enabled"))
	}
	metricsConfig := readConfigSection(cfg, config.OTLPMetrics)
	debugConfig := readConfigSection(cfg, config.OTLPDebug)

	return PipelineConfig{
		OTLPReceiverConfig: otlpConfig.ToStringMap(),
		TracePort:          tracePort,
		MetricsEnabled:     metricsEnabled,
		TracesEnabled:      tracesEnabled,
		Metrics:            metricsConfig.ToStringMap(),
		Debug:              debugConfig.ToStringMap(),
	}, multierr.Combine(errs...)
}

// IsEnabled checks if OTLP pipeline is enabled in a given config.
func IsEnabled(cfg config.Config) bool {
	// HACK: We want to mark as enabled if the section is present, even if empty, so that we get errors
	// from unmarshaling/validation done by the Collector code.
	//
	// IsSet won't work here: it will return false if the section is present but empty.
	// To work around this, we check if the receiver key is present in the string map, which does the 'correct' thing.
	_, ok := readConfigSection(cfg, config.OTLPSection).ToStringMap()[config.OTLPReceiverSubSectionKey]
	return ok
}

// IsDisplayed checks if the OTLP section should be rendered in the Agent
func IsDisplayed() bool {
	return true
}
