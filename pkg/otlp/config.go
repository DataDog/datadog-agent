// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package otlp

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
	"go.opentelemetry.io/collector/consumer/consumererror"
)

const (
	experimentalHTTPPortSetting  = "experimental.otlp.http_port"
	experimentalgRPCPortSetting  = "experimental.otlp.grpc_port"
	experimentalTracePortSetting = "experimental.otlp.internal_traces_port"
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

// isSetExperimental checks if the experimental config is set.
func isSetExperimental(cfg config.Config) bool {
	return cfg.IsSet(experimentalHTTPPortSetting) || cfg.IsSet(experimentalgRPCPortSetting)
}

func portToUint(v int) (port uint, err error) {
	if v < 0 || v > 65535 {
		err = fmt.Errorf("%d is out of [0, 65535] range", v)
	}
	port = uint(v)
	return
}

// fromExperimentalConfig builds a PipelineConfig from the experimental configuration.
func fromExperimentalConfig(cfg config.Config) (PipelineConfig, error) {
	var errs []error

	httpPort, err := portToUint(cfg.GetInt(experimentalHTTPPortSetting))
	if err != nil {
		errs = append(errs, fmt.Errorf("http port is invalid: %w", err))
	}

	gRPCPort, err := portToUint(cfg.GetInt(experimentalgRPCPortSetting))
	if err != nil {
		errs = append(errs, fmt.Errorf("gRPC port is invalid: %w", err))
	}

	tracePort, err := portToUint(cfg.GetInt(experimentalTracePortSetting))
	if err != nil {
		errs = append(errs, fmt.Errorf("internal trace port is invalid: %w", err))
	}

	return PipelineConfig{
		BindHost:  getReceiverHost(cfg),
		HTTPPort:  httpPort,
		GRPCPort:  gRPCPort,
		TracePort: tracePort,
	}, consumererror.Combine(errs)
}

// IsEnabled checks if OTLP pipeline is enabled in a given config.
func IsEnabled(cfg config.Config) bool {
	// TODO (AP-1267): Check stable config too
	return isSetExperimental(cfg)
}

// FromAgentConfig builds a pipeline configuration from an Agent configuration.
func FromAgentConfig(cfg config.Config) (PipelineConfig, error) {
	// TODO (AP-1267): Check stable config too
	return fromExperimentalConfig(cfg)
}
