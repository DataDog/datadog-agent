// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package setup

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// OTLP configuration paths.
const (
	OTLPSection               = "otlp_config"
	OTLPTracePort             = OTLPSection + ".traces.internal_port"
	OTLPTracesEnabled         = OTLPSection + ".traces.enabled"
	OTLPLogsEnabled           = OTLPSection + ".logs.enabled"
	OTLPReceiverSubSectionKey = "receiver"
	OTLPReceiverSection       = OTLPSection + "." + OTLPReceiverSubSectionKey
	OTLPMetrics               = OTLPSection + ".metrics"
	OTLPMetricsEnabled        = OTLPMetrics + ".enabled"
	OTLPDebug                 = OTLPSection + "." + "debug"
)

// OTLP related configuration.
func OTLP(config pkgconfigmodel.Setup) {
	config.BindEnv("otlp_config.grpc_port") // TODO OTLP team: add default value
	config.BindEnv("otlp_config.http_port") // TODO OTLP team: add default value

	// NOTE: This only partially works.
	// The environment variable is also manually checked in comp/otelcol/otlp/config.go
	config.BindEnvAndSetDefault("otlp_config.metrics.tag_cardinality", "low", "DD_OTLP_TAG_CARDINALITY")

	// Logs
	config.BindEnvAndSetDefault("otlp_config.logs.enabled", false)

	// Traces settings
	config.BindEnvAndSetDefault("otlp_config.traces.enabled", true)
	config.BindEnvAndSetDefault("otlp_config.traces.span_name_as_resource_name", false)
	config.BindEnvAndSetDefault("otlp_config.traces.span_name_remappings", map[string]string{})
	config.BindEnvAndSetDefault("otlp_config.traces.ignore_missing_datadog_fields", false, "DD_OTLP_CONFIG_IGNORE_MISSING_DATADOG_FIELDS")
	config.BindEnvAndSetDefault("otlp_config.traces.probabilistic_sampler.sampling_percentage", 100.,
		"DD_OTLP_CONFIG_TRACES_PROBABILISTIC_SAMPLER_SAMPLING_PERCENTAGE")
	config.BindEnvAndSetDefault("otlp_config.traces.internal_port", 5003)

	// TODO(OTAGENT-378): Fix OTLP ingestion configs so that they can have default values
	// For now do NOT add default values for any config under otlp_config.receiver, that will force the OTLP ingestion pipelines to always start

	// gRPC settings
	config.BindEnv("otlp_config.receiver.protocols.grpc.endpoint")
	config.BindEnv("otlp_config.receiver.protocols.grpc.transport")
	config.BindEnv("otlp_config.receiver.protocols.grpc.max_recv_msg_size_mib")
	config.BindEnv("otlp_config.receiver.protocols.grpc.max_concurrent_streams")
	config.BindEnv("otlp_config.receiver.protocols.grpc.read_buffer_size")
	config.BindEnv("otlp_config.receiver.protocols.grpc.write_buffer_size")
	config.BindEnv("otlp_config.receiver.protocols.grpc.include_metadata")
	config.BindEnv("otlp_config.receiver.protocols.grpc.keepalive.enforcement_policy.min_time")

	// HTTP settings
	config.BindEnv("otlp_config.receiver.protocols.http.endpoint")
	config.BindEnv("otlp_config.receiver.protocols.http.max_request_body_size")
	config.BindEnv("otlp_config.receiver.protocols.http.include_metadata")
	config.BindEnv("otlp_config.receiver.protocols.http.cors.allowed_headers")
	config.BindEnv("otlp_config.receiver.protocols.http.cors.allowed_origins")

	// Metrics settings
	config.BindEnv("otlp_config.metrics.tags") // TODO OTLP team: add default value
	config.BindEnvAndSetDefault("otlp_config.metrics.enabled", true)
	config.BindEnv("otlp_config.metrics.resource_attributes_as_tags")             // TODO OTLP team: add default value
	config.BindEnv("otlp_config.metrics.instrumentation_scope_metadata_as_tags")  // TODO OTLP team: add default value
	config.BindEnv("otlp_config.metrics.tag_cardinality")                         // TODO OTLP team: add default value
	config.BindEnv("otlp_config.metrics.delta_ttl")                               // TODO OTLP team: add default value
	config.BindEnv("otlp_config.metrics.histograms.mode")                         // TODO OTLP team: add default value
	config.BindEnv("otlp_config.metrics.histograms.send_count_sum_metrics")       // TODO OTLP team: add default value
	config.BindEnv("otlp_config.metrics.histograms.send_aggregation_metrics")     // TODO OTLP team: add default value
	config.BindEnv("otlp_config.metrics.sums.cumulative_monotonic_mode")          // TODO OTLP team: add default value
	config.BindEnv("otlp_config.metrics.sums.initial_cumulative_monotonic_value") // TODO OTLP team: add default value
	config.BindEnv("otlp_config.metrics.summaries.mode")                          // TODO OTLP team: add default value

	// Debug settings
	config.BindEnv("otlp_config.debug.verbosity")
}
