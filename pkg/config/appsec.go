// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

// setupAppSec initializes the configuration values of the appsec agent.
func setupAppSec(cfg Config) {
	cfg.BindEnv("appsec_config.enabled", "DD_APPSEC_ENABLED")
	cfg.BindEnv("appsec_config.appsec_dd_url", "DD_APPSEC_DD_URL")
	cfg.BindEnv("appsec_config.max_payload_size", "DD_APPSEC_MAX_PAYLOAD_SIZE")
	cfg.BindEnv("appsec_config.obfuscation.parameter_key_regexp", "DD_APPSEC_OBFUSCATION_PARAMETER_KEY_REGEXP")
	cfg.BindEnv("appsec_config.obfuscation.parameter_value_regexp", "DD_APPSEC_OBFUSCATION_PARAMETER_VALUE_REGEXP")
}
