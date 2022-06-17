// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

// setupEvpProxy initializes the configuration values of the evp_proxy.
func setupEvpProxy(cfg Config) {
	cfg.BindEnv("evp_proxy_config.enabled")
	cfg.BindEnv("evp_proxy_config.dd_url")
	cfg.BindEnv("evp_proxy_config.api_key")
	cfg.BindEnv("evp_proxy_config.additional_endpoints")
	cfg.BindEnv("evp_proxy_config.max_payload_size")
}
