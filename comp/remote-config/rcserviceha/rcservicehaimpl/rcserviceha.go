// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package rcservicehaimpl is a remote config service that can run within the agent to receive remote config updates from the configured DD failover DC
package rcservicehaimpl

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/log"

	cfgcomp "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcserviceha"
	"github.com/DataDog/datadog-agent/comp/remote-config/rctelemetryreporter"
	"github.com/DataDog/datadog-agent/pkg/config"
	remoteconfig "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/pkg/version"

	"go.uber.org/fx"
)

// Module conditionally provides the HA DC remote config service.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newHaRemoteConfigServiceOptional),
	)
}

type dependencies struct {
	fx.In

	Lc fx.Lifecycle

	DdRcTelemetryReporter rctelemetryreporter.Component
	Hostname              hostname.Component
	Cfg                   cfgcomp.Component
	Logger                log.Component
}

// newHaRemoteConfigServiceOptional conditionally creates and configures a new HA remote config service, based on whether RC is enabled.
func newHaRemoteConfigServiceOptional(deps dependencies) optional.Option[rcserviceha.Component] {
	none := optional.NewNoneOption[rcserviceha.Component]()
	if !config.IsRemoteConfigEnabled(deps.Cfg) || !deps.Cfg.GetBool("ha.enabled") {
		return none
	}

	haConfigService, err := newHaRemoteConfigService(deps)
	if err != nil {
		deps.Logger.Errorf("remote config HA service not initialized or started: %s", err)
		return none
	}

	return optional.NewOption[rcserviceha.Component](haConfigService)
}

// newHaRemoteConfigServiceOptional creates and configures a new service that receives remote config updates from the configured DD failover DC
func newHaRemoteConfigService(deps dependencies) (rcserviceha.Component, error) {
	apiKey := configUtils.SanitizeAPIKey(deps.Cfg.GetString("ha.api_key"))
	baseRawURL, err := configUtils.GetHAEndpoint(deps.Cfg, "https://config.", "ha.remote_configuration.rc_dd_url")
	if err != nil {
		return nil, fmt.Errorf("unable to get HA remote config endpoint: %s", err)
	}
	traceAgentEnv := configUtils.GetTraceAgentDefaultEnv(deps.Cfg)
	configuredTags := configUtils.GetConfiguredTags(deps.Cfg, false)
	options := []remoteconfig.Option{
		remoteconfig.WithAPIKey(apiKey),
		remoteconfig.WithTraceAgentEnv(traceAgentEnv),
		remoteconfig.WithDatabaseFileName("remote-config-ha.db"),
		remoteconfig.WithConfigRootOverride(deps.Cfg.GetString("ha.remote_configuration.config_root")),
		remoteconfig.WithDirectorRootOverride(deps.Cfg.GetString("ha.remote_configuration.director_root")),
		remoteconfig.WithRcKey(deps.Cfg.GetString("ha.remote_configuration.key")),
	}
	if deps.Cfg.IsSet("ha.remote_configuration.refresh_interval") {
		options = append(options, remoteconfig.WithRefreshInterval(deps.Cfg.GetDuration("ha.remote_configuration.refresh_interval"), "ha.remote_configuration.refresh_interval"))
	}
	if deps.Cfg.IsSet("ha.remote_configuration.max_backoff_interval") {
		options = append(options, remoteconfig.WithMaxBackoffInterval(deps.Cfg.GetDuration("ha.remote_configuration.max_backoff_interval"), "remote_configuration.max_backoff_time"))
	}
	if deps.Cfg.IsSet("ha.remote_configuration.clients.ttl_seconds") {
		options = append(options, remoteconfig.WithClientTTL(deps.Cfg.GetDuration("ha.remote_configuration.clients.ttl_seconds"), "ha.remote_configuration.clients.ttl_seconds"))
	}
	if deps.Cfg.IsSet("ha.remote_configuration.clients.cache_bypass_limit") {
		options = append(options, remoteconfig.WithClientCacheBypassLimit(deps.Cfg.GetInt("ha.remote_configuration.clients.cache_bypass_limit"), "ha.remote_configuration.clients.cache_bypass_limit"))
	}

	haConfigService, err := remoteconfig.NewService(
		deps.Cfg,
		"HA",
		baseRawURL,
		deps.Hostname.GetSafe(context.Background()),
		configuredTags,
		deps.DdRcTelemetryReporter,
		version.AgentVersion,
		options...,
	)
	if err != nil {
		return nil, fmt.Errorf("unable to create HA remote-config service: %w", err)
	}

	deps.Lc.Append(fx.Hook{OnStart: func(_ context.Context) error {
		haConfigService.Start()
		deps.Logger.Info("remote config HA service started")
		return nil
	}})
	deps.Lc.Append(fx.Hook{OnStop: func(_ context.Context) error {
		deps.Logger.Info("remote config HA service stopped")
		err = haConfigService.Stop()
		if err != nil {
			deps.Logger.Errorf("unable to stop remote config HA service: %s", err)
			return err
		}
		return nil
	}})

	return haConfigService, nil
}
