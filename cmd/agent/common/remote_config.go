// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.
// TODO https://datadoghq.atlassian.net/browse/RC-1453 Remove this file once the remote config service is refactored

package common

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/config"
	remoteconfig "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// NewRemoteConfigService returns a new remote configuration service
func NewRemoteConfigService(hostname string, telemetryReporter *DdRcTelemetryReporter) (*remoteconfig.Service, error) {
	apiKey := config.Datadog.GetString("api_key")
	if config.Datadog.IsSet("remote_configuration.api_key") {
		apiKey = config.Datadog.GetString("remote_configuration.api_key")
	}
	apiKey = configUtils.SanitizeAPIKey(apiKey)
	baseRawURL := configUtils.GetMainEndpoint(config.Datadog, "https://config.", "remote_configuration.rc_dd_url")
	traceAgentEnv := configUtils.GetTraceAgentDefaultEnv(config.Datadog)
	dbName := "remote-config.db"

	configService, err := remoteconfig.NewService(config.Datadog, apiKey, baseRawURL, hostname, telemetryReporter, version.AgentVersion, dbName, remoteconfig.WithTraceAgentEnv(traceAgentEnv))
	if err != nil {
		return nil, fmt.Errorf("unable to create remote-config service: %w", err)
	}

	return configService, nil
}

// NewHARemoteConfigService returns a new remote configuration service that uses the failover DC endpoint
func NewHARemoteConfigService(hostname string, telemetryReporter *DdRcTelemetryReporter) (*remoteconfig.Service, error) {
	apiKey := configUtils.SanitizeAPIKey(config.Datadog.GetString("ha.api_key"))
	baseRawURL := configUtils.GetHAEndpoint(config.Datadog, "https://config.", "ha.rc_dd_url")
	traceAgentEnv := configUtils.GetTraceAgentDefaultEnv(config.Datadog)
	dbName := "remote-config-ha.db"

	configService, err := remoteconfig.NewService(config.Datadog, apiKey, baseRawURL, hostname, telemetryReporter, version.AgentVersion, dbName, remoteconfig.WithTraceAgentEnv(traceAgentEnv))
	if err != nil {
		return nil, fmt.Errorf("unable to create HA remote-config service: %w", err)
	}

	return configService, nil
}

// DdRcTelemetryReporter is a datadog-agent telemetry counter for RC cache bypass metrics. It implements the RcTelemetryReporter interface.
type DdRcTelemetryReporter struct {
	BypassRateLimitCounter telemetry.Counter
	BypassTimeoutCounter   telemetry.Counter
}

// IncRateLimit increments the ddRcTelemetryReporter BypassRateLimitCounter counter.
func (r *DdRcTelemetryReporter) IncRateLimit() {
	r.BypassRateLimitCounter.Inc()
}

// IncTimeout increments the ddRcTelemetryReporter BypassTimeoutCounter counter.
func (r *DdRcTelemetryReporter) IncTimeout() {
	r.BypassTimeoutCounter.Inc()
}

// NewRcTelemetryReporter returns a new ddRcTelemetryReporter that uses the datadog-agent telemetry package to emit metrics.
func NewRcTelemetryReporter() *DdRcTelemetryReporter {
	commonOpts := telemetry.Options{NoDoubleUnderscoreSep: true}
	return &DdRcTelemetryReporter{
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
}
