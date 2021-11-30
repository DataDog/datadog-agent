// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package config

// Experimental OTLP configuration paths.
const (
	ExperimentalOTLPSection         = "experimental.otlp"
	ExperimentalOTLPHTTPPort        = ExperimentalOTLPSection + ".http_port"
	ExperimentalOTLPgRPCPort        = ExperimentalOTLPSection + ".grpc_port"
	ExperimentalOTLPTracePort       = ExperimentalOTLPSection + ".internal_traces_port"
	ExperimentalOTLPMetricsEnabled  = ExperimentalOTLPSection + ".metrics_enabled"
	ExperimentalOTLPTracesEnabled   = ExperimentalOTLPSection + ".traces_enabled"
	ReceiverSubSectionKey           = "receiver"
	ExperimentalOTLPReceiverSection = ExperimentalOTLPSection + "." + ReceiverSubSectionKey
	ExperimentalOTLPMetrics         = ExperimentalOTLPSection + ".metrics"
)

// SetupOTLP related configuration.
func SetupOTLP(config Config) {
	config.BindEnvAndSetDefault(ExperimentalOTLPTracePort, 5003)
	config.BindEnvAndSetDefault(ExperimentalOTLPMetricsEnabled, true)
	config.BindEnvAndSetDefault(ExperimentalOTLPTracesEnabled, true)
	config.BindEnv(ExperimentalOTLPHTTPPort, "DD_OTLP_HTTP_PORT")
	config.BindEnv(ExperimentalOTLPgRPCPort, "DD_OTLP_GRPC_PORT")

	config.SetKnown(ExperimentalOTLPMetrics)
	// Set all subkeys of experimental.otlp.metrics as known
	config.SetKnown(ExperimentalOTLPMetrics + ".*")
	config.SetKnown(ExperimentalOTLPReceiverSection)
	// Set all subkeys of experimental.otlp.receiver as known
	config.SetKnown(ExperimentalOTLPReceiverSection + ".*")
}
