// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

func setupHAMR(config Config) {
	config.BindEnv("ha.api_key")
	config.BindEnv("ha.site")
	config.BindEnv("ha.dd_url")
	config.BindEnvAndSetDefault("ha.enabled", false)
	config.BindEnvAndSetDefault("ha.failover", false)
}
