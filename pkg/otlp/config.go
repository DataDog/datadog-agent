// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build !serverless && otlp
// +build !serverless,otlp

package otlp

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
	colConfig "go.opentelemetry.io/collector/config"
	"go.uber.org/multierr"
)

// getReceiverHost gets the receiver host for the OTLP endpoint in a given config.
func getReceiverHost(cfg config.Config) (receiverHost string) {
	// The default value for the trace Agent
	receiverHost = "localhost"

	// This is taken from pkg/trace/config.AgentConfig.applyDatadogConfig
	if cfg.IsSet("bind_host") || cfg.IsSet("apm_config.apm_non_local_traffic") {
		if cfg.IsSet("bind_host") {
			receiverHost = cfg.GetString("bind_host")
		}

		if cfg.IsSet("apm_config.apm_non_local_traffic") && cfg.GetBool("apm_config.apm_non_local_traffic") {
			receiverHost = "0.0.0.0"
		}
	} else if config.IsContainerized() {
		receiverHost = "0.0.0.0"
	}
	return
}

// isSetMetrics checks if the metrics config is set.
func isSetMetrics(cfg config.Config) bool {
	return cfg.IsSet(config.OTLPMetrics)
}

func portToUint(v int) (port uint, err error) {
	if v < 0 || v > 65535 {
		err = fmt.Errorf("%d is out of [0, 65535] range", v)
	}
	port = uint(v)
	return
}

func fromReceiverSectionConfig(cfg config.Config) *colConfig.Map {
	return colConfig.NewMapFromStringMap(
		cfg.GetStringMap(config.OTLPReceiverSection),
	)
}

// fromConfig builds a PipelineConfig from the configuration.
func fromConfig(cfg config.Config) (PipelineConfig, error) {
	var errs []error
	otlpConfig := fromReceiverSectionConfig(cfg)

	tracePort, err := portToUint(cfg.GetInt(config.OTLPTracePort))
	if err != nil {
		errs = append(errs, fmt.Errorf("internal trace port is invalid: %w", err))
	}

	metricsEnabled := cfg.GetBool(config.OTLPMetricsEnabled)
	tracesEnabled := cfg.GetBool(config.OTLPTracesEnabled)
	if !metricsEnabled && !tracesEnabled {
		errs = append(errs, fmt.Errorf("at least one OTLP signal needs to be enabled"))
	}

	metrics := map[string]interface{}{}
	if isSetMetrics(cfg) {
		metrics = cfg.GetStringMap(config.OTLPMetrics)
	}

	return PipelineConfig{
		OTLPReceiverConfig: otlpConfig.ToStringMap(),
		TracePort:          tracePort,
		MetricsEnabled:     metricsEnabled,
		TracesEnabled:      tracesEnabled,
		Metrics:            metrics,
	}, multierr.Combine(errs...)
}

// IsEnabled checks if OTLP pipeline is enabled in a given config.
func IsEnabled(cfg config.Config) bool {
	// HACK: We want to mark as enabled if the section is present, even if empty, so that we get errors
	// from unmarshaling/validation done by the Collector code.
	//
	// IsSet won't work here: it will return false if the section is present but empty.
	// To work around this, we check if the receiver key is present in the string map, which does the 'correct' thing.
	_, ok := cfg.GetStringMap(config.OTLPSection)[config.OTLPReceiverSubSectionKey]
	return ok
}

// FromAgentConfig builds a pipeline configuration from an Agent configuration.
func FromAgentConfig(cfg config.Config) (PipelineConfig, error) {
	// TODO (AP-1267): Check stable config too
	return fromConfig(cfg)
}
