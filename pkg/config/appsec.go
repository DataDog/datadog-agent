// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import traceconfig "github.com/DataDog/datadog-agent/pkg/trace/config"

// setupAppSec initializes the configuration values of the appsec agent.
func setupAppSec(cfg Config) {
	cfg.BindEnvAndSetDefault("appsec_config.enabled", traceconfig.DefaultAppSecEnabled, "DD_APPSEC_ENABLED")
	cfg.BindEnvAndSetDefault("appsec_config.appsec_dd_url", traceconfig.DefaultAppSecDDUrl, "DD_APPSEC_DD_URL")
	cfg.BindEnvAndSetDefault("appsec_config.max_payload_size", traceconfig.DefaultAppSecMaxPayloadSize, "DD_APPSEC_MAX_PAYLOAD_SIZE")
	cfg.BindEnvAndSetDefault("appsec_config.obfuscation.parameter_key_regexp", traceconfig.DefaultAppSecObfuscationKeyRegexp, "DD_APPSEC_OBFUSCATION_PARAMETER_KEY_REGEXP")
	cfg.BindEnvAndSetDefault("appsec_config.obfuscation.parameter_value_regexp", traceconfig.DefaultAppSecObfuscationValueRegexp, "DD_APPSEC_OBFUSCATION_PARAMETER_VALUE_REGEXP")
}
