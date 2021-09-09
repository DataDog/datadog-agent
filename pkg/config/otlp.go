// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package config

func setupOTLP(config Config) {
	config.BindEnvAndSetDefault("experimental.otlp.internal_traces_port", 5003)
	config.BindEnv("experimental.otlp.http_port", "DD_OTLP_HTTP_PORT")
	config.BindEnv("experimental.otlp.grpc_port", "DD_OTLP_GRPC_PORT")
}
