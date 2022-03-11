// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

// setupAppSec initializes the configuration values of the appsec agent.
func setupAppSec(cfg Config) {
	cfg.BindEnvAndSetDefault("appsec_config.enabled", true, "DD_APPSEC_ENABLED")
	cfg.BindEnvAndSetDefault("appsec_config.appsec_dd_url", "", "DD_APPSEC_DD_URL")
	cfg.BindEnvAndSetDefault("appsec_config.max_payload_size", 5*1024*1024, "DD_APPSEC_MAX_PAYLOAD_SIZE")
}
