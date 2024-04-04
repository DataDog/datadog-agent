// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package rcserviceimpl is a remote config service that can run within the agent to receive remote config updates from the DD backend.
package rcserviceimpl

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"

	cfgcomp "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservice"
	"github.com/DataDog/datadog-agent/comp/remote-config/rctelemetryreporter"
	"github.com/DataDog/datadog-agent/pkg/config"
	remoteconfig "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/version"

	"go.uber.org/fx"
)

// Module conditionally provides the remote config service.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newRemoteConfigServiceOptional),
	)
}

type dependencies struct {
	fx.In

	Lc fx.Lifecycle

	Params                *rcservice.Params `optional:"true"`
	DdRcTelemetryReporter rctelemetryreporter.Component
	Hostname              hostname.Component
	Cfg                   cfgcomp.Component
	Logger                log.Component
}

// newRemoteConfigServiceOptional conditionally creates and configures a new remote config service, based on whether RC is enabled.
func newRemoteConfigServiceOptional(deps dependencies) optional.Option[rcservice.Component] {
	none := optional.NewNoneOption[rcservice.Component]()
	if !config.IsRemoteConfigEnabled(deps.Cfg) {
		return none
	}

	configService, err := newRemoteConfigService(deps)
	if err != nil {
		deps.Logger.Errorf("remote config service not initialized or started: %s", err)
		return none
	}

	return optional.NewOption[rcservice.Component](configService)
}

// newRemoteConfigServiceOptional creates and configures a new remote config service
func newRemoteConfigService(deps dependencies) (rcservice.Component, error) {
	apiKey := deps.Cfg.GetString("api_key")
	if deps.Cfg.IsSet("remote_configuration.api_key") {
		apiKey = deps.Cfg.GetString("remote_configuration.api_key")
	}
	apiKey = configUtils.SanitizeAPIKey(apiKey)
	baseRawURL := configUtils.GetMainEndpoint(deps.Cfg, "https://config.", "remote_configuration.rc_dd_url")
	traceAgentEnv := configUtils.GetTraceAgentDefaultEnv(deps.Cfg)
	configuredTags := configUtils.GetConfiguredTags(deps.Cfg, false)

	options := []remoteconfig.Option{
		remoteconfig.WithAPIKey(apiKey),
		remoteconfig.WithTraceAgentEnv(traceAgentEnv),
		remoteconfig.WithConfigRootOverride(deps.Cfg.GetString("remote_configuration.config_root")),
		remoteconfig.WithDirectorRootOverride(deps.Cfg.GetString("remote_configuration.director_root")),
		remoteconfig.WithRcKey(deps.Cfg.GetString("remote_configuration.key")),
	}
	if deps.Params != nil {
		options = append(options, deps.Params.Options...)
	}
	if deps.Cfg.IsSet("remote_configuration.refresh_interval") {
		options = append(options, remoteconfig.WithRefreshInterval(deps.Cfg.GetDuration("remote_configuration.refresh_interval"), "remote_configuration.refresh_interval"))
	}
	if deps.Cfg.IsSet("remote_configuration.max_backoff_interval") {
		options = append(options, remoteconfig.WithMaxBackoffInterval(deps.Cfg.GetDuration("remote_configuration.max_backoff_interval"), "remote_configuration.max_backoff_interval"))
	}
	if deps.Cfg.IsSet("remote_configuration.clients.ttl_seconds") {
		options = append(options, remoteconfig.WithClientTTL(deps.Cfg.GetDuration("remote_configuration.clients.ttl_seconds"), "remote_configuration.clients.ttl_seconds"))
	}
	if deps.Cfg.IsSet("remote_configuration.clients.cache_bypass_limit") {
		options = append(options, remoteconfig.WithClientCacheBypassLimit(deps.Cfg.GetInt("remote_configuration.clients.cache_bypass_limit"), "remote_configuration.clients.cache_bypass_limit"))
	}

	configService, err := remoteconfig.NewService(
		deps.Cfg,
		"Remote Config",
		baseRawURL,
		deps.Hostname.GetSafe(context.Background()),
		configuredTags,
		deps.DdRcTelemetryReporter,
		version.AgentVersion,
		options...,
	)
	if err != nil {
		return nil, fmt.Errorf("unable to create remote config service: %w", err)
	}

	deps.Lc.Append(fx.Hook{OnStart: func(_ context.Context) error {
		configService.Start()
		deps.Logger.Info("remote config service started")
		return nil
	}})
	deps.Lc.Append(fx.Hook{OnStop: func(_ context.Context) error {
		err = configService.Stop()
		if err != nil {
			deps.Logger.Errorf("unable to stop remote config service: %s", err)
			return err
		}
		deps.Logger.Info("remote config service stopped")
		return nil
	}})

	return configService, nil
}
