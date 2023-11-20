// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

func setupHAMR(config Config) {
	config.BindEnv("ha.api_key", "DD_HA_API_KEY")
	config.BindEnv("ha.site", "DD_HA_SITE")
	config.BindEnvAndSetDefault("ha.enabled", false, "DD_HA_ENABLED")
	config.BindEnvAndSetDefault("ha.failover", false, "DD_HA_FAILOVER")
}
