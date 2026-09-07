// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package rcclient

import (
	"fmt"

	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
	rcservice "github.com/DataDog/datadog-agent/comp/remote-config/rcservice/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	client "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	remoteconfig "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const databaseFileName = "par-remote-config.db"

// HasBackendOverride reports whether PAR needs a Remote Config service separate
// from the resident Agent's service.
func HasBackendOverride(cfg model.Reader) bool {
	return cfg.GetString(setup.PARSite) != "" || cfg.GetString(setup.PARAPIKey) != ""
}

// ServiceParams configures the standalone PAR's Remote Config service with the
// same backend selection used for enrollment, OPMS, and telemetry.
func ServiceParams(cfg configcomp.Component) *rcservice.Params {
	return serviceParams(cfg)
}

func serviceParams(cfg model.Reader) *rcservice.Params {
	if !HasBackendOverride(cfg) {
		return &rcservice.Params{}
	}
	backend := parconfig.ResolveBackend(cfg)
	baseURL := configutils.GetMainEndpoint(cfg, "https://config.", "remote_configuration.rc_dd_url")
	if cfg.GetString(setup.PARSite) != "" {
		baseURL = configutils.BuildURLWithPrefix("https://config.", backend.Site)
	}
	apiKeyPath := "api_key"
	if cfg.GetString(setup.PARAPIKey) != "" {
		apiKeyPath = setup.PARAPIKey
	}
	return &rcservice.Params{
		BaseURLOverride: baseURL,
		Options: []remoteconfig.Option{
			remoteconfig.WithAPIKey(configutils.SanitizeAPIKey(backend.APIKey)),
			remoteconfig.WithAPIKeyPath(apiKeyPath),
			remoteconfig.WithConfigRootOverride(backend.Site, cfg.GetString("remote_configuration.config_root")),
			remoteconfig.WithDirectorRootOverride(backend.Site, cfg.GetString("remote_configuration.director_root")),
		},
	}
}

// NewBackend creates a dedicated Remote Config service/client pair for an
// embedded PAR. Keeping this service separate prevents PAR's site and API key
// from retargeting unrelated Cluster Agent Remote Config products.
func NewBackend(cfg model.Reader, hostname, clusterName, clusterID string) (*client.Client, func() error, error) {
	backend := parconfig.ResolveBackend(cfg)
	baseURL := configutils.GetMainEndpoint(cfg, "https://config.", "remote_configuration.rc_dd_url")
	if cfg.GetString(setup.PARSite) != "" {
		baseURL = configutils.BuildURLWithPrefix("https://config.", backend.Site)
	}

	apiKeyPath := "api_key"
	if cfg.GetString(setup.PARAPIKey) != "" {
		apiKeyPath = setup.PARAPIKey
	}
	service, err := remoteconfig.NewService(
		cfg,
		"Private Action Runner Remote Config",
		baseURL,
		hostname,
		func() []string { return nil },
		noopTelemetryReporter{},
		version.AgentVersion,
		remoteconfig.WithAPIKey(configutils.SanitizeAPIKey(backend.APIKey)),
		remoteconfig.WithAPIKeyPath(apiKeyPath),
		remoteconfig.WithConfigRootOverride(backend.Site, cfg.GetString("remote_configuration.config_root")),
		remoteconfig.WithDirectorRootOverride(backend.Site, cfg.GetString("remote_configuration.director_root")),
		remoteconfig.WithDatabaseFileName(databaseFileName),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("create PAR Remote Config service: %w", err)
	}

	rcClient, err := client.NewClient(
		service,
		client.WithAgent("private-action-runner", version.AgentVersion),
		client.WithCluster(clusterName, clusterID),
		client.WithDirectorRootOverride(backend.Site, cfg.GetString("remote_configuration.director_root")),
	)
	if err != nil {
		_ = service.Stop()
		return nil, nil, fmt.Errorf("create PAR Remote Config client: %w", err)
	}

	service.Start()
	rcClient.Start()
	stop := func() error {
		rcClient.Close()
		return service.Stop()
	}
	return rcClient, stop, nil
}

type noopTelemetryReporter struct{}

func (noopTelemetryReporter) IncTimeout()                                {}
func (noopTelemetryReporter) IncRateLimit()                              {}
func (noopTelemetryReporter) IncConfigSubscriptionsConnectedCounter()    {}
func (noopTelemetryReporter) IncConfigSubscriptionsDisconnectedCounter() {}
func (noopTelemetryReporter) SetConfigSubscriptionsActive(int)           {}
func (noopTelemetryReporter) SetConfigSubscriptionClientsTracked(int)    {}
