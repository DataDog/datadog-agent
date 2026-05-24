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
	OTLPSection                = "otlp_config"
	OTLPTracePort              = OTLPSection + ".traces.internal_port"
	OTLPTracesEnabled          = OTLPSection + ".traces.enabled"
	OTLPTracesInfraAttrEnabled = OTLPSection + ".traces.infra_attributes.enabled"

	OTLPLogs        = OTLPSection + ".logs"
	OTLPLogsEnabled = OTLPLogs + ".enabled"

	OTLPReceiverSubSectionKey = "receiver"
	OTLPReceiverSection       = OTLPSection + "." + OTLPReceiverSubSectionKey

	OTLPMetrics        = OTLPSection + ".metrics"
	OTLPMetricsEnabled = OTLPMetrics + ".enabled"
	OTLPMetricsBatch   = OTLPMetrics + ".batch"

	OTLPDebug = OTLPSection + "." + "debug"

	DataPlaneSection     = "data_plane"
	DataPlaneEnabled     = DataPlaneSection + ".enabled"
	DataPlaneOTLPSection = DataPlaneSection + ".otlp"
	DataPlaneOTLPEnabled = DataPlaneOTLPSection + ".enabled"

	DataPlaneOTLPProxySection = DataPlaneOTLPSection + ".proxy"
	DataPlaneOTLPProxyEnabled = DataPlaneOTLPProxySection + ".enabled"

	DataPlaneOTLPProxyReceiverSection               = DataPlaneOTLPProxySection + ".receiver"
	DataPlaneOTLPProxyReceiverProtocolsGRPCEndpoint = DataPlaneOTLPProxyReceiverSection + ".protocols.grpc.endpoint"
)

// OTLP related configuration.
func OTLP(config pkgconfigmodel.Setup) {
	// Legacy port keys (unused; 0 = disabled)
	config.BindEnvAndSetDefault("otlp_config.grpc_port", 0)
	config.BindEnvAndSetDefault("otlp_config.http_port", 0)

	// NOTE: This only partially works.
	// The environment variable is also manually checked in comp/otelcol/otlp/config.go
	config.BindEnvAndSetDefault("otlp_config.metrics.tag_cardinality", "low",
		"DD_OTLP_CONFIG_METRICS_TAG_CARDINALITY", "DD_OTLP_TAG_CARDINALITY")

	// Logs
	config.BindEnvAndSetDefault("otlp_config.logs.enabled", false)
	config.BindEnvAndSetDefault("otlp_config.logs.batch.min_size", 8192)
	config.BindEnvAndSetDefault("otlp_config.logs.batch.max_size", 0)
	config.BindEnvAndSetDefault("otlp_config.logs.batch.flush_timeout", "200ms")

	// Traces settings
	config.BindEnvAndSetDefault("otlp_config.traces.enabled", true)
	config.BindEnvAndSetDefault("otlp_config.traces.span_name_as_resource_name", false)
	config.BindEnvAndSetDefault("otlp_config.traces.span_name_remappings", map[string]string{})
	config.BindEnvAndSetDefault("otlp_config.traces.probabilistic_sampler.sampling_percentage", 100.,
		"DD_OTLP_CONFIG_TRACES_PROBABILISTIC_SAMPLER_SAMPLING_PERCENTAGE")
	config.BindEnvAndSetDefault("otlp_config.traces.internal_port", 5003)

	// Receiver gRPC settings (defaults from OTel otlpreceiver / configgrpc)
	config.BindEnvAndSetDefault("otlp_config.receiver.protocols.grpc.endpoint", "localhost:4317")
	config.BindEnvAndSetDefault("otlp_config.receiver.protocols.grpc.transport", "tcp")
	config.BindEnvAndSetDefault("otlp_config.receiver.protocols.grpc.max_recv_msg_size_mib", 0)
	config.BindEnvAndSetDefault("otlp_config.receiver.protocols.grpc.max_concurrent_streams", 0)
	config.BindEnvAndSetDefault("otlp_config.receiver.protocols.grpc.read_buffer_size", 524288)
	config.BindEnvAndSetDefault("otlp_config.receiver.protocols.grpc.write_buffer_size", 0)
	config.BindEnvAndSetDefault("otlp_config.receiver.protocols.grpc.include_metadata", false)
	config.BindEnvAndSetDefault("otlp_config.receiver.protocols.grpc.keepalive.enforcement_policy.min_time", "5m")

	// Receiver HTTP settings (defaults from OTel otlpreceiver / confighttp)
	config.BindEnvAndSetDefault("otlp_config.receiver.protocols.http.endpoint", "localhost:4318")
	config.BindEnvAndSetDefault("otlp_config.receiver.protocols.http.max_request_body_size", 0)
	config.BindEnvAndSetDefault("otlp_config.receiver.protocols.http.include_metadata", false)
	config.BindEnvAndSetDefault("otlp_config.receiver.protocols.http.cors.allowed_headers", []string{})
	config.BindEnvAndSetDefault("otlp_config.receiver.protocols.http.cors.allowed_origins", []string{})

	// Metrics settings
	config.BindEnvAndSetDefault("otlp_config.metrics.tags", "")
	config.BindEnvAndSetDefault("otlp_config.metrics.enabled", true)
	config.BindEnvAndSetDefault("otlp_config.metrics.resource_attributes_as_tags", false)
	config.BindEnvAndSetDefault("otlp_config.metrics.instrumentation_scope_metadata_as_tags", true)
	config.BindEnvAndSetDefault("otlp_config.metrics.delta_ttl", 3600)
	config.BindEnvAndSetDefault("otlp_config.metrics.histograms.mode", "distributions")
	config.BindEnvAndSetDefault("otlp_config.metrics.histograms.send_count_sum_metrics", false)
	config.BindEnvAndSetDefault("otlp_config.metrics.histograms.send_aggregation_metrics", false)
	config.BindEnvAndSetDefault("otlp_config.metrics.sums.cumulative_monotonic_mode", "to_delta")
	config.BindEnvAndSetDefault("otlp_config.metrics.sums.initial_cumulative_monotonic_value", "auto")
	config.BindEnvAndSetDefault("otlp_config.metrics.summaries.mode", "gauges")
	config.BindEnvAndSetDefault("otlp_config.metrics.batch.min_size", 8192)
	config.BindEnvAndSetDefault("otlp_config.metrics.batch.max_size", 0)
	config.BindEnvAndSetDefault("otlp_config.metrics.batch.flush_timeout", "200ms")

	config.BindEnvAndSetDefault("otlp_config.traces.infra_attributes.enabled", true)

	// Debug settings (default from OTel debugexporter)
	config.BindEnvAndSetDefault("otlp_config.debug.verbosity", "basic")
}
