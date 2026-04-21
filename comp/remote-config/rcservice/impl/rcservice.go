// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package rcserviceimpl is a remote config service that can run within the agent to receive remote config updates from the DD backend.
package rcserviceimpl

import (
	"context"
	"expvar"
	"fmt"
	"strings"
	"time"

	cfgcomp "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggertags "github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl/hosttags"
	rcservice "github.com/DataDog/datadog-agent/comp/remote-config/rcservice/def"
	rctelemetryreporter "github.com/DataDog/datadog-agent/comp/remote-config/rctelemetryreporter/def"
	remoteconfig "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var (
	rcExpvars              = expvar.NewMap("remoteConfigStartup")
	rcStartupFailureReason = expvar.String{}
)

func init() {
	rcExpvars.Init()
	rcExpvars.Set("startupFailureReason", &rcStartupFailureReason)
}

// Dependencies defines the dependencies for the rcservice component.
type Dependencies struct {
	compdef.In

	Lc compdef.Lifecycle

	Params                *rcservice.Params `optional:"true"`
	DdRcTelemetryReporter rctelemetryreporter.Component
	Hostname              hostname.Component
	Cfg                   cfgcomp.Component
	Logger                log.Component
	Tagger                option.Option[tagger.Component]
}

// Provides defines the output of the rcservice component.
type Provides struct {
	compdef.Out

	Comp option.Option[rcservice.Component]
}

// NewRemoteConfigServiceOptional conditionally creates and configures a new remote config service, based on whether RC is enabled.
func NewRemoteConfigServiceOptional(deps Dependencies) Provides {
	none := option.None[rcservice.Component]()
	if !configUtils.IsRemoteConfigEnabled(deps.Cfg) {
		return Provides{Comp: none}
	}

	configService, err := newRemoteConfigService(deps)
	if err != nil {
		deps.Logger.Errorf("remote config service not initialized or started: %s", err)
		return Provides{Comp: none}
	}

	return Provides{Comp: option.New[rcservice.Component](configService)}
}

// newRemoteConfigService creates and configures a new remote config service
func newRemoteConfigService(deps Dependencies) (rcservice.Component, error) {
	apiKey := deps.Cfg.GetString("api_key")
	if deps.Cfg.IsSet("remote_configuration.api_key") {
		apiKey = deps.Cfg.GetString("remote_configuration.api_key")
	}
	apiKey = configUtils.SanitizeAPIKey(apiKey)

	baseRawURL := configUtils.GetMainEndpoint(deps.Cfg, "https://config.", "remote_configuration.rc_dd_url")
	traceAgentEnv := configUtils.GetTraceAgentDefaultEnv(deps.Cfg)

	options := []remoteconfig.Option{
		remoteconfig.WithAPIKey(apiKey),
		remoteconfig.WithTraceAgentEnv(traceAgentEnv),
		remoteconfig.WithConfigRootOverride(deps.Cfg.GetString("site"), deps.Cfg.GetString("remote_configuration.config_root")),
		remoteconfig.WithDirectorRootOverride(deps.Cfg.GetString("site"), deps.Cfg.GetString("remote_configuration.director_root")),
		remoteconfig.WithRcKey(deps.Cfg.GetString("remote_configuration.key")),
	}
	if deps.Params != nil {
		options = append(options, deps.Params.Options...)
	}
	if deps.Cfg.IsSet("remote_configuration.refresh_interval") {
		options = append(options, remoteconfig.WithRefreshInterval(deps.Cfg.GetDuration("remote_configuration.refresh_interval"), "remote_configuration.refresh_interval"))
	}
	if deps.Cfg.IsSet("remote_configuration.org_status_refresh_interval") {
		options = append(options, remoteconfig.WithOrgStatusRefreshInterval(deps.Cfg.GetDuration("remote_configuration.org_status_refresh_interval"), "remote_configuration.org_status_refresh_interval"))
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
		getTags(deps.Cfg, deps.Tagger),
		deps.DdRcTelemetryReporter,
		version.AgentVersion,
		options...,
	)
	if err != nil {
		rcStartupFailureReason.Set(err.Error())
		return nil, fmt.Errorf("unable to create remote config service: %w", err)
	}
	rcStartupFailureReason.Set("")

	deps.Lc.Append(compdef.Hook{OnStart: func(_ context.Context) error {
		configService.Start()
		deps.Logger.Info("remote config service started")
		return nil
	}})
	deps.Lc.Append(compdef.Hook{OnStop: func(_ context.Context) error {
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

func getTags(config cfgcomp.Component, taggerOpt option.Option[tagger.Component]) func() []string {
	return func() []string {
		// Host tags are cached on host, but we add a timeout to avoid blocking the RC request
		// if the host tags are not available yet and need to be fetched. They will be fetched
		// by the first agent metadata V5 payload.
		ctx, cc := context.WithTimeout(context.Background(), time.Second)
		defer cc()
		hostTags := hosttags.Get(ctx, true, config)
		tags := append(hostTags.System, hostTags.GoogleCloudPlatform...)

		// On ECS Fargate, the task_arn tag is not part of host tags but is
		// needed for RC predicate targeting. Fetch it from the tagger's global
		// tags at orchestrator cardinality.
		if taggerComp, ok := taggerOpt.Get(); ok {
			globalTags, err := taggerComp.GlobalTags(taggertypes.OrchestratorCardinality)
			if err == nil {
				taskARNPrefix := taggertags.TaskARN + ":"
				for _, t := range globalTags {
					if strings.HasPrefix(t, taskARNPrefix) {
						tags = append(tags, t)
						break
					}
				}
			}
		}

		return tags
	}
}
