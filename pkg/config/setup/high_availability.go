// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"time"
)

func setupHighAvailability(config pkgconfigmodel.Config) {
	config.BindEnv("ha.api_key")
	config.BindEnv("ha.site")
	config.BindEnv("ha.dd_url")
	config.BindEnvAndSetDefault("ha.enabled", false)
	config.BindEnvAndSetDefault("ha.failover", false)

	config.BindEnv("ha.remote_configuration.refresh_interval")
	config.BindEnv("ha.remote_configuration.max_backoff_time")
	config.BindEnvAndSetDefault("ha.remote_configuration.max_backoff_interval", 5*time.Minute)
	config.BindEnv("ha.remote_configuration.config_root")
	config.BindEnv("ha.remote_configuration.director_root")
	config.BindEnv("ha.remote_configuration.key")

	config.BindEnvAndSetDefault("ha.remote_configuration.clients.ttl_seconds", 30*time.Second)
	config.BindEnvAndSetDefault("ha.remote_configuration.clients.cache_bypass_limit", 5)
}
