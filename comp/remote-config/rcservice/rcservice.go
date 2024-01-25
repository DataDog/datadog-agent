package rcservice

import (
	"context"
	"fmt"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	"github.com/DataDog/datadog-agent/comp/remote-config/rctelemetryreporter"
	"github.com/DataDog/datadog-agent/pkg/config"
	remoteconfig "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/version"
	"go.uber.org/fx"
)

// team: remote-config

type dependencies struct {
	fx.In

	DdRcTelemetryReporter rctelemetryreporter.Component
	Hostname              hostname.Component
}

// newRemoteConfigService creates and configures a new remote config service.
func newRemoteConfigService(deps dependencies) (Component, error) {
	// TODO is there a better way to do this utilizing fx to conditionally initialize a dep?
	if !config.IsRemoteConfigEnabled(config.Datadog) {
		return &remoteconfig.Service{}, nil
	}

	apiKey := config.Datadog.GetString("api_key")
	if config.Datadog.IsSet("remote_configuration.api_key") {
		apiKey = config.Datadog.GetString("remote_configuration.api_key")
	}
	apiKey = configUtils.SanitizeAPIKey(apiKey)
	baseRawURL := configUtils.GetMainEndpoint(config.Datadog, "https://config.", "remote_configuration.rc_dd_url")
	traceAgentEnv := configUtils.GetTraceAgentDefaultEnv(config.Datadog)
	configuredTags := configUtils.GetConfiguredTags(config.Datadog, false)

	configService, err := remoteconfig.NewService(
		config.Datadog,
		apiKey,
		baseRawURL,
		deps.Hostname.GetSafe(context.Background()),
		configuredTags,
		deps.DdRcTelemetryReporter,
		version.AgentVersion,
		remoteconfig.WithTraceAgentEnv(traceAgentEnv),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to create remote-config service: %w", err)
	}

	return configService, nil
}
