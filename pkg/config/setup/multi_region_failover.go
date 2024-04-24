// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"time"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func setupMultiRegionFailover(config pkgconfigmodel.Config) {
	config.BindEnv("multi_region_failover.api_key")
	config.BindEnv("multi_region_failover.site")
	config.BindEnv("multi_region_failover.dd_url")
	config.BindEnvAndSetDefault("multi_region_failover.enabled", false)
	config.BindEnvAndSetDefault("multi_region_failover.failover_metrics", false)
	config.BindEnvAndSetDefault("multi_region_failover.failover_logs", false)

	config.BindEnv("multi_region_failover.remote_configuration.refresh_interval")
	config.BindEnv("multi_region_failover.remote_configuration.max_backoff_time")
	config.BindEnvAndSetDefault("multi_region_failover.remote_configuration.max_backoff_interval", 5*time.Minute)
	config.BindEnv("multi_region_failover.remote_configuration.config_root")
	config.BindEnv("multi_region_failover.remote_configuration.director_root")
	config.BindEnv("multi_region_failover.remote_configuration.key")

	config.BindEnvAndSetDefault("multi_region_failover.remote_configuration.clients.ttl_seconds", 30*time.Second)
	config.BindEnvAndSetDefault("multi_region_failover.remote_configuration.clients.cache_bypass_limit", 5)
}
