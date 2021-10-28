// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package config

// Experimental OTLP configuration paths.
const (
	experimentalOTLPPrefix         = "experimental.otlp"
	ExperimentalOTLPHTTPPort       = experimentalOTLPPrefix + ".http_port"
	ExperimentalOTLPgRPCPort       = experimentalOTLPPrefix + ".grpc_port"
	ExperimentalOTLPTracePort      = experimentalOTLPPrefix + ".internal_traces_port"
	ExperimentalOTLPMetricsEnabled = experimentalOTLPPrefix + ".metrics_enabled"
	ExperimentalOTLPTracesEnabled  = experimentalOTLPPrefix + ".traces_enabled"

	experimentalOTLPMetricsPrefix                               = experimentalOTLPPrefix + ".metrics"
	ExperimentalOTLPMetricsQuantiles                            = experimentalOTLPMetricsPrefix + ".report_quantiles"
	ExperimentalOTLPMetricsSendMonotonic                        = experimentalOTLPMetricsPrefix + ".send_monotonic_counter"
	ExperimentalOTLPMetricsDeltaTTL                             = experimentalOTLPMetricsPrefix + ".delta_ttl"
	ExperimentalOTLPMetricsResourceAttributesAsTags             = experimentalOTLPMetricsPrefix + ".resource_attributes_as_tags"
	ExperimentalOTLPMetricsInstrumentationLibraryMetadataAsTags = experimentalOTLPMetricsPrefix + ".instrumentation_library_metadata_as_tags"

	experimentalOTLPMetricsHistogramsPrefix       = experimentalOTLPMetricsPrefix + ".histograms"
	ExperimentalOTLPMetricsHistogramsSendCountSum = experimentalOTLPMetricsHistogramsPrefix + ".send_count_sum_metrics"
	ExperimentalOTLPMetricsHistogramsMode         = experimentalOTLPMetricsHistogramsPrefix + ".mode"
)

// SetupOTLP related configuration.
func SetupOTLP(config Config) {
	config.BindEnvAndSetDefault(ExperimentalOTLPTracePort, 5003)
	config.BindEnvAndSetDefault(ExperimentalOTLPMetricsEnabled, true)
	config.BindEnvAndSetDefault(ExperimentalOTLPTracesEnabled, true)

	config.BindEnvAndSetDefault(ExperimentalOTLPMetricsQuantiles, true)
	config.BindEnvAndSetDefault(ExperimentalOTLPMetricsSendMonotonic, false)
	config.BindEnvAndSetDefault(ExperimentalOTLPMetricsDeltaTTL, 3600)
	config.BindEnvAndSetDefault(ExperimentalOTLPMetricsResourceAttributesAsTags, false)
	config.BindEnvAndSetDefault(ExperimentalOTLPMetricsInstrumentationLibraryMetadataAsTags, false)

	config.BindEnvAndSetDefault(ExperimentalOTLPMetricsHistogramsSendCountSum, false)
	config.BindEnvAndSetDefault(ExperimentalOTLPMetricsHistogramsMode, "distributions")

	config.BindEnv(ExperimentalOTLPHTTPPort, "DD_OTLP_HTTP_PORT")
	config.BindEnv(ExperimentalOTLPgRPCPort, "DD_OTLP_GRPC_PORT")
}
