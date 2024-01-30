// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package rcservicehaimpl is a remote config service that can run within the agent to receive remote config updates from the configured DD failover DC
package rcservicehaimpl

import (
	"context"
	"fmt"

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
}

// newHaRemoteConfigServiceOptional conditionally creates and configures a new HA remote config service, based on whether RC is enabled.
func newHaRemoteConfigServiceOptional(deps dependencies) (optional.Option[rcserviceha.Component], error) {
	none := optional.NewNoneOption[rcserviceha.Component]()
	if !config.IsRemoteConfigEnabled(deps.Cfg) || !deps.Cfg.GetBool("ha.enabled") {
		return none, nil
	}

	haConfigService, err := newHaRemoteConfigService(deps)
	if err != nil {
		return none, err
	}

	return optional.NewOption[rcserviceha.Component](haConfigService), nil
}

// newHaRemoteConfigServiceOptional creates and configures a new service that receives remote config updates from the configured DD failover DC
func newHaRemoteConfigService(deps dependencies) (rcserviceha.Component, error) {
	apiKey := configUtils.SanitizeAPIKey(config.Datadog.GetString("ha.api_key"))
	baseRawURL := configUtils.GetHAEndpoint(config.Datadog, "https://config.", "ha.rc_dd_url")
	traceAgentEnv := configUtils.GetTraceAgentDefaultEnv(config.Datadog)
	dbName := "remote-config-ha.db"
	configuredTags := configUtils.GetConfiguredTags(config.Datadog, false)

	haConfigService, err := remoteconfig.NewService(
		config.Datadog,
		apiKey,
		baseRawURL,
		deps.Hostname.GetSafe(context.Background()),
		configuredTags,
		deps.DdRcTelemetryReporter,
		version.AgentVersion,
		dbName,
		remoteconfig.WithTraceAgentEnv(traceAgentEnv),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to create HA remote-config service: %w", err)
	}

	deps.Lc.Append(fx.Hook{OnStart: func(ctx context.Context) error {
		haConfigService.Start(ctx)
		return nil
	}})
	deps.Lc.Append(fx.Hook{OnStop: func(context.Context) error {
		return haConfigService.Stop()
	}})

	return haConfigService, nil
}
