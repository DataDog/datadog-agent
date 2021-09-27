// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package config

// Experimental OTLP configuration paths.
const (
	experimentalOTLPPrefix    = "experimental.otlp"
	ExperimentalOTLPHTTPPort  = experimentalOTLPPrefix + ".http_port"
	ExperimentalOTLPgRPCPort  = experimentalOTLPPrefix + ".grpc_port"
	ExperimentalOTLPTracePort = experimentalOTLPPrefix + ".internal_traces_port"
)

// SetupOTLP related configuration.
func SetupOTLP(config Config) {
	config.BindEnvAndSetDefault(ExperimentalOTLPTracePort, 5003)
	config.BindEnv(ExperimentalOTLPHTTPPort, "DD_OTLP_HTTP_PORT")
	config.BindEnv(ExperimentalOTLPgRPCPort, "DD_OTLP_GRPC_PORT")
}
