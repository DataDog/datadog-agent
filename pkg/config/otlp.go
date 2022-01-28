// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package config

// OTLP configuration paths.
const (
	OTLPSection               = "otlp"
	OTLPHTTPPort              = OTLPSection + ".http_port"
	OTLPgRPCPort              = OTLPSection + ".grpc_port"
	OTLPTracePort             = OTLPSection + ".internal_traces_port"
	OTLPMetricsEnabled        = OTLPSection + ".metrics_enabled"
	OTLPTracesEnabled         = OTLPSection + ".traces_enabled"
	OTLPReceiverSubSectionKey = "receiver"
	OTLPReceiverSection       = OTLPSection + "." + OTLPReceiverSubSectionKey
	OTLPMetrics               = OTLPSection + ".metrics"
	OTLPTagCardinalityKey     = OTLPMetrics + ".tag_cardinality"
)

// SetupOTLP related configuration.
func SetupOTLP(config Config) {
	config.BindEnvAndSetDefault(OTLPTracePort, 5003)
	config.BindEnvAndSetDefault(OTLPMetricsEnabled, true)
	config.BindEnvAndSetDefault(OTLPTracesEnabled, true)
	config.BindEnv(OTLPHTTPPort, "DD_OTLP_HTTP_PORT")
	config.BindEnv(OTLPgRPCPort, "DD_OTLP_GRPC_PORT")

	// NOTE: This only partially works.
	// The environment variable is also manually checked in pkg/otlp/config.go
	config.BindEnvAndSetDefault(OTLPTagCardinalityKey, "low", "DD_OTLP_TAG_CARDINALITY")

	config.SetKnown(OTLPMetrics)
	// Set all subkeys of otlp.metrics as known
	config.SetKnown(OTLPMetrics + ".*")
	config.SetKnown(OTLPReceiverSection)
	// Set all subkeys of otlp.receiver as known
	config.SetKnown(OTLPReceiverSection + ".*")
}
