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
	if !config.IsRemoteConfigEnabled(deps.Cfg) && (deps.Params == nil || !deps.Params.ForceEnabled) {
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
	apiKey := config.Datadog.GetString("api_key")
	if config.Datadog.IsSet("remote_configuration.api_key") {
		apiKey = config.Datadog.GetString("remote_configuration.api_key")
	}
	apiKey = configUtils.SanitizeAPIKey(apiKey)
	baseRawURL := configUtils.GetMainEndpoint(config.Datadog, "https://config.", "remote_configuration.rc_dd_url")
	traceAgentEnv := configUtils.GetTraceAgentDefaultEnv(config.Datadog)
	configuredTags := configUtils.GetConfiguredTags(config.Datadog, false)

	options := []remoteconfig.Option{
		remoteconfig.WithTraceAgentEnv(traceAgentEnv),
	}
	if deps.Params != nil {
		options = append(options, deps.Params.Options...)
	}

	configService, err := remoteconfig.NewService(
		config.Datadog,
		apiKey,
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
