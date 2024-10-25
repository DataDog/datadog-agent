// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/remote-config/rctelemetryreporter/rctelemetryreporterimpl"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	remoteconfig "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
)

func StartRCService(functionARN string) *remoteconfig.CoreAgentService {
	config := pkgconfigsetup.Datadog()
	if pkgconfigsetup.IsRemoteConfigEnabled(config) {
		config.Set("run_path", "/tmp/datadog-agent", model.SourceAgentRuntime)
		apiKey := config.GetString("api_key")
		if config.IsSet("remote_configuration.api_key") {
			apiKey = config.GetString("remote_configuration.api_key")
		}
		apiKey = configUtils.SanitizeAPIKey(apiKey)
		baseRawURL := configUtils.GetMainEndpoint(config, "https://config.", "remote_configuration.rc_dd_url")
		traceAgentEnv := configUtils.GetTraceAgentDefaultEnv(config)

		options := []remoteconfig.Option{
			remoteconfig.WithAPIKey(apiKey),
			remoteconfig.WithTraceAgentEnv(traceAgentEnv),
			remoteconfig.WithConfigRootOverride(config.GetString("site"), config.GetString("remote_configuration.config_root")),
			remoteconfig.WithDirectorRootOverride(config.GetString("site"), config.GetString("remote_configuration.director_root")),
			remoteconfig.WithRcKey(config.GetString("remote_configuration.key")),
		}

		if config.IsSet("remote_configuration.refresh_interval") {
			options = append(options, remoteconfig.WithRefreshInterval(config.GetDuration("remote_configuration.refresh_interval"), "remote_configuration.refresh_interval"))
		}
		if config.IsSet("remote_configuration.max_backoff_interval") {
			options = append(options, remoteconfig.WithMaxBackoffInterval(config.GetDuration("remote_configuration.max_backoff_interval"), "remote_configuration.max_backoff_interval"))
		}
		if config.IsSet("remote_configuration.clients.ttl_seconds") {
			options = append(options, remoteconfig.WithClientTTL(config.GetDuration("remote_configuration.clients.ttl_seconds"), "remote_configuration.clients.ttl_seconds"))
		}
		if config.IsSet("remote_configuration.clients.cache_bypass_limit") {
			options = append(options, remoteconfig.WithClientCacheBypassLimit(config.GetInt("remote_configuration.clients.cache_bypass_limit"), "remote_configuration.clients.cache_bypass_limit"))
		}
		tagsGetter := func() []string {
			arn, parseErr := arn.Parse(functionARN)
			if parseErr != nil {
				return []string{}
			}
			return []string{fmt.Sprintf("aws_account_id:%s", arn.AccountID), fmt.Sprintf("region:%s", arn.Region)}
		}
		commonOpts := telemetry.Options{NoDoubleUnderscoreSep: true}
		telemetryReporter := &rctelemetryreporterimpl.DdRcTelemetryReporter{
			BypassRateLimitCounter: telemetry.NewCounterWithOpts(
				"remoteconfig",
				"cache_bypass_ratelimiter_skip",
				[]string{},
				"Number of Remote Configuration cache bypass requests skipped by rate limiting.",
				commonOpts,
			),
			BypassTimeoutCounter: telemetry.NewCounterWithOpts(
				"remoteconfig",
				"cache_bypass_timeout",
				[]string{},
				"Number of Remote Configuration cache bypass requests that timeout.",
				commonOpts,
			),
		}

		configService, err := remoteconfig.NewService(
			config,
			"Remote Config",
			baseRawURL,
			"",
			tagsGetter,
			telemetryReporter,
			version.AgentVersion,
			options...,
		)
		if err != nil {
			log.Errorf("unable to create remote config service: %v", err)
			return nil
		}
		configService.Start()
		return configService
	} else {
		log.Debug("Remote configuration configuration is disabled, did not create service")
		return nil
	}
}
