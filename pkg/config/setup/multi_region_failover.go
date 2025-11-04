// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"time"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func setupMultiRegionFailover(config pkgconfigmodel.Setup) {
	config.BindEnv("multi_region_failover.api_key")                //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	config.BindEnv("multi_region_failover.site")                   //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	config.BindEnv("multi_region_failover.dd_url")                 //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	config.BindEnv("multi_region_failover.metric_allowlist")       //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	config.BindEnv("multi_region_failover.logs_service_allowlist") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	config.BindEnvAndSetDefault("multi_region_failover.enabled", false)
	config.BindEnvAndSetDefault("multi_region_failover.failover_metrics", false)
	config.BindEnvAndSetDefault("multi_region_failover.failover_logs", false)
	config.BindEnvAndSetDefault("multi_region_failover.failover_apm", false)

	config.BindEnv("multi_region_failover.remote_configuration.refresh_interval") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	config.BindEnvAndSetDefault("multi_region_failover.remote_configuration.org_status_refresh_interval", 1*time.Minute)
	config.BindEnv("multi_region_failover.remote_configuration.max_backoff_time") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	config.BindEnvAndSetDefault("multi_region_failover.remote_configuration.max_backoff_interval", 5*time.Minute)
	config.BindEnv("multi_region_failover.remote_configuration.config_root")   //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	config.BindEnv("multi_region_failover.remote_configuration.director_root") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	config.BindEnv("multi_region_failover.remote_configuration.key")           //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'

	config.BindEnvAndSetDefault("multi_region_failover.remote_configuration.clients.ttl_seconds", 30*time.Second)
	config.BindEnvAndSetDefault("multi_region_failover.remote_configuration.clients.cache_bypass_limit", 5)
}
