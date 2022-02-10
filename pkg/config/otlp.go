// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package config

import (
	"net"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// OTLP configuration paths.
const (
	OTLPSection               = "otlp_config"
	OTLPTracesSubSectionKey   = "traces"
	OTLPTracePort             = OTLPSection + "." + OTLPTracesSubSectionKey + ".internal_port"
	OTLPTracesEnabled         = OTLPSection + "." + OTLPTracesSubSectionKey + ".enabled"
	OTLPReceiverSubSectionKey = "receiver"
	OTLPReceiverSection       = OTLPSection + "." + OTLPReceiverSubSectionKey
	OTLPMetricsSubSectionKey  = "metrics"
	OTLPMetrics               = OTLPSection + "." + OTLPMetricsSubSectionKey
	OTLPMetricsEnabled        = OTLPSection + "." + OTLPMetricsSubSectionKey + ".enabled"
	OTLPTagCardinalityKey     = OTLPMetrics + ".tag_cardinality"
)

// SetupOTLP related configuration.
func SetupOTLP(config Config) {
	config.BindEnvAndSetDefault(OTLPTracePort, 5003)
	config.BindEnvAndSetDefault(OTLPMetricsEnabled, true)
	config.BindEnvAndSetDefault(OTLPTracesEnabled, true)

	// NOTE: This only partially works.
	// The environment variable is also manually checked in pkg/otlp/config.go
	config.BindEnvAndSetDefault(OTLPTagCardinalityKey, "low", "DD_OTLP_TAG_CARDINALITY")

	config.SetKnown(OTLPMetrics)
	// Set all subkeys of otlp.metrics as known
	config.SetKnown(OTLPMetrics + ".*")
	config.SetKnown(OTLPReceiverSection)
	// Set all subkeys of otlp.receiver as known
	config.SetKnown(OTLPReceiverSection + ".*")
	// set environment variables for selected fields
	setupOTLPEnvironmentVariables(config)
}

// promoteExperimentalOTLP checks if "experimental.otlp" is set and promotes it to the top level
// "otlp_config" configuration if unset.
//
// TODO(gbbr): This is to keep backwards compatibility and should
// be completely removed once 7.35.0 is out.
func promoteExperimentalOTLP(cfg Config) {
	if !cfg.IsSet("experimental.otlp") {
		return
	}
	log.Warn(`OTLP ingest configuration is now stable and has been moved out of the "experimental" section. ` +
		`This section will be deprecated in a future Datadog Agent release. Please use the "otlp_config" section instead.`)
	if k := "experimental.otlp.metrics"; cfg.IsSet(k) {
		for key, val := range cfg.GetStringMap(k) {
			cfg.Set(OTLPMetrics+"."+key, val)
		}
	}
	if k := "experimental.otlp.metrics_enabled"; cfg.IsSet(k) {
		cfg.Set(OTLPMetricsEnabled, cfg.GetBool(k))
	}
	if k := "experimental.otlp.tag_cardinality"; cfg.IsSet(k) {
		cfg.Set(OTLPTagCardinalityKey, cfg.GetString(k))
	}
	if k := "experimental.otlp.traces_enabled"; cfg.IsSet(k) {
		cfg.Set(OTLPTracesEnabled, cfg.GetBool(k))
	}
	if v := cfg.GetString("experimental.otlp.internal_traces_port"); v != "" {
		cfg.Set(OTLPTracePort, v)
	}
	if v, ok := cfg.GetStringMap("experimental.otlp")[OTLPReceiverSubSectionKey]; ok {
		if v == nil {
			cfg.Set(OTLPReceiverSection, nil)
		} else {
			for key, val := range cfg.GetStringMap("experimental.otlp.receiver") {
				cfg.Set(OTLPReceiverSection+"."+key, val)
			}
		}
	}
	if v := cfg.GetString("experimental.otlp.http_port"); v != "" {
		cfg.Set(OTLPReceiverSection+".protocols.http.endpoint", net.JoinHostPort(getBindHost(cfg), v))
	}
	if v := cfg.GetString("experimental.otlp.grpc_port"); v != "" {
		cfg.Set(OTLPReceiverSection+".protocols.grpc.endpoint", net.JoinHostPort(getBindHost(cfg), v))
	}
}

// setupOTLPEnvironmentVariables sets up the environment variables associated with different OTLP ingest settings:
// If there are changes in the OTLP receiver configuration, they should be reflected here.
//
// We don't need to set the default value: it is dealt with at the unmarshaling level
// since we get the configuration through GetStringMap
//
// We are missing TLS settings: since some of them need more work to work right they are not included here.
func setupOTLPEnvironmentVariables(config Config) {
	// gRPC settings
	config.BindEnv(OTLPSection + ".receiver.protocols.grpc.endpoint")
	config.BindEnv(OTLPSection + ".receiver.protocols.grpc.transport")
	config.BindEnv(OTLPSection + ".receiver.protocols.grpc.max_recv_msg_size_mib")
	config.BindEnv(OTLPSection + ".receiver.protocols.grpc.max_concurrent_streams")
	config.BindEnv(OTLPSection + ".receiver.protocols.grpc.read_buffer_size")
	config.BindEnv(OTLPSection + ".receiver.protocols.grpc.write_buffer_size")
	config.BindEnv(OTLPSection + ".receiver.protocols.grpc.include_metadata")

	// HTTP settings
	config.BindEnv(OTLPSection + ".receiver.protocols.http.endpoint")
	config.BindEnv(OTLPSection + ".receiver.protocols.http.max_request_body_size")
	config.BindEnv(OTLPSection + ".receiver.protocols.http.include_metadata")

	// Metrics settings
	config.BindEnv(OTLPSection + ".metrics.report_quantiles")
	config.BindEnv(OTLPSection + ".metrics.send_monotonic_counter")
	config.BindEnv(OTLPSection + ".metrics.delta_ttl")
	config.BindEnv(OTLPSection + ".metrics.resource_attributes_as_tags")
	config.BindEnv(OTLPSection + ".metrics.instrumentation_library_metadata_as_tags")
	config.BindEnv(OTLPSection + ".metrics.tag_cardinality")
	config.BindEnv(OTLPSection + ".metrics.histograms.mode")
	config.BindEnv(OTLPSection + ".metrics.histograms.send_count_sum_metrics")
}
