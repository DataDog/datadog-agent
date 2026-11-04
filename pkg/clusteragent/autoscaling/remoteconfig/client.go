// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remoteconfig

import (
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	remoteconfig "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// pollInterval matches the Cluster Agent's default remote configuration client.
const pollInterval = 5 * time.Second

// Client is the autoscaling product's own Remote Configuration client. It owns
// the service behind it, so starting and stopping the client does both, and it
// is an rcclient.Client everywhere else.
type Client struct {
	*rcclient.Client
	service  *remoteconfig.CoreAgentService
	status   *EndpointStatus
	settings Settings
}

// Start begins polling, on the service and the client both. Nothing polls until
// it is called: the client notifies only the listeners registered when an update
// arrives and never replays, so the caller subscribes first.
func (c *Client) Start() {
	c.service.Start()
	c.status.MarkStarted()
	c.Client.Start()
	pkglog.Infof("Autoscaling reads its configuration from %s (site %s)", c.settings.URL, c.settings.Site)
}

// Close stops the client and the service behind it
func (c *Client) Close() {
	c.Client.Close()
	if err := c.service.Stop(); err != nil {
		pkglog.Errorf("Unable to stop the autoscaling remote configuration service: %s", err)
	}
}

// NewClient builds the autoscaling product's own Remote Configuration client
// against the configured endpoint, subscribed only to the given products. It
// returns the client idle; the caller subscribes its listeners and then calls
// Start.
func NewClient(cfg model.Reader, hostname, clusterName, clusterID string, products []string, endpointStatus *EndpointStatus) (*Client, error) {
	if endpointStatus == nil {
		// The service reports into this unconditionally.
		endpointStatus = NewEndpointStatus()
	}

	settings, err := Load(cfg)
	if err != nil {
		return nil, err
	}
	if len(products) == 0 {
		return nil, errors.New("no autoscaling remote configuration products to subscribe to")
	}
	if clusterID == "" {
		// Targeting is by self-reported cluster ID: without one the backend
		// never matches this client.
		return nil, fmt.Errorf("cluster ID is required to use %s", URLSetting)
	}

	service, err := remoteconfig.NewService(
		cfg,
		"Autoscaling",
		settings.URL,
		hostname,
		clusterAgentTags(cfg),
		noopTelemetryReporter{},
		version.AgentVersion,
		remoteconfig.WithAPIKey(settings.APIKey),
		remoteconfig.WithTraceAgentEnv(configUtils.GetTraceAgentDefaultEnv(cfg)),
		// Shares run_path with the default service, so it needs its own cache.
		remoteconfig.WithDatabaseFileName(DatabaseFileName),
		remoteconfig.WithConfigRootOverride(settings.Site, settings.ConfigRoot),
		remoteconfig.WithDirectorRootOverride(settings.Site, settings.DirectorRoot),
		remoteconfig.WithRcKey(settings.Key),
		remoteconfig.WithStatusInstance(endpointStatus),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to create the autoscaling remote configuration service for %s: %w", settings.URL, err)
	}

	client, err := rcclient.NewClient(service,
		rcclient.WithAgent("cluster-agent", version.AgentVersion),
		rcclient.WithCluster(clusterName, clusterID),
		rcclient.WithDirectorRootOverride(settings.Site, settings.DirectorRoot),
		rcclient.WithProducts(products...),
		rcclient.WithPollInterval(pollInterval),
	)
	if err != nil {
		if stopErr := service.Stop(); stopErr != nil {
			pkglog.Errorf("Unable to stop the autoscaling remote configuration service: %s", stopErr)
		}
		return nil, fmt.Errorf("unable to create the autoscaling remote configuration client for %s: %w", settings.URL, err)
	}

	return &Client{
		Client:   client,
		service:  service,
		status:   endpointStatus,
		settings: settings,
	}, nil
}

// clusterAgentTags reports the Cluster Agent's configured tags, so backend
// predicate targeting can match on them. The default service additionally sends
// host tags and the ECS task ARN, so targeting is not identical on the two.
func clusterAgentTags(cfg model.Reader) func() []string {
	return func() []string {
		tags := configUtils.GetConfiguredTags(cfg, false)
		return append(tags, configUtils.GetConfiguredDCATags(cfg)...)
	}
}

// noopTelemetryReporter keeps this client out of the process-wide remote
// configuration telemetry, which describes the default endpoint.
type noopTelemetryReporter struct{}

func (noopTelemetryReporter) IncRateLimit()                              {}
func (noopTelemetryReporter) IncTimeout()                                {}
func (noopTelemetryReporter) IncConfigSubscriptionsConnectedCounter()    {}
func (noopTelemetryReporter) IncConfigSubscriptionsDisconnectedCounter() {}
func (noopTelemetryReporter) SetConfigSubscriptionsActive(_ int)         {}
func (noopTelemetryReporter) SetConfigSubscriptionClientsTracked(_ int)  {}
