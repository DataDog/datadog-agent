// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

// Package datadogclientimpl implements datadog client component for querying external metrics.
package datadogclientimpl

import (
	"sync"

	datadogclient "github.com/DataDog/datadog-agent/comp/autoscaling/datadogclient/def"
	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.uber.org/fx"
	"gopkg.in/zorkian/go-datadog-api.v2"
)

const (
	metricsEndpointPrefix          = "https://api."
	metricsEndpointConfig          = "external_metrics_provider.endpoint"
	metricsRedundantEndpointConfig = "external_metrics_provider.endpoints"
)

// endpoint represent a datadog endpoint
type endpoint struct {
	Site   string `mapstructure:"site" json:"site" yaml:"site"`
	URL    string `mapstructure:"url" json:"url" yaml:"url"`
	APIKey string `mapstructure:"api_key" json:"api_key" yaml:"api_key"`
	APPKey string `mapstructure:"app_key" json:"app_key" yaml:"app_key"`
}

type dependencies struct {
	fx.In
	Config configComponent.Component
}

// datadogClient is a wrapper around the datadog.Client, which allows for
// refresh of the client pointer in case of app/api key changes
type datadogClient struct {
	client        datadogclient.Component
	mux           sync.RWMutex
	datadogConfig configComponent.Component
}

var _ datadogclient.Component = (*datadog.Client)(nil) // client implemented by zorkian/go-datadog-api.v2
var _ datadogclient.Component = (*datadogClient)(nil)  // ensure wrapper: datadogClient implements the interface

// QueryMetrics takes as input from, to (seconds from Unix Epoch) and query string and then requests
// timeseries data for that time peried
func (d *datadogClient) QueryMetrics(from, to int64, query string) ([]datadog.Series, error) {
	d.mux.RLock()
	defer d.mux.RUnlock()
	return d.client.QueryMetrics(from, to, query)
}

// GetRateLimitStats is a threadsafe getter to retrieve the rate limiting stats associated with the Client.
func (d *datadogClient) GetRateLimitStats() map[string]datadog.RateLimit {
	d.mux.RLock()
	defer d.mux.RUnlock()
	return d.client.GetRateLimitStats()
}

func (d *datadogClient) refreshClient() {
	d.mux.Lock()
	defer d.mux.Unlock()
	newClient, err := createDatadogClient(d.datadogConfig)
	if err != nil {
		log.Errorf("error refreshing datadog client: %v", err)
		return
	}
	log.Infof("refreshed datadog client")
	d.client = newClient
}

func createDatadogClient(cfg configComponent.Component) (datadogclient.Component, error) {
	if cfg.IsSet(metricsRedundantEndpointConfig) {
		var endpoints []endpoint
		if err := config.Datadog().UnmarshalKey(metricsRedundantEndpointConfig, &endpoints); err != nil {
			return nil, log.Errorf("could not parse %s: %v", metricsRedundantEndpointConfig, err)
		}

		return newDatadogFallbackClient(cfg, endpoints)
	}
	return newDatadogSingleClient(cfg)
}

// NewDatadogClient configures and returns a new DatadogClient
func NewDatadogClient(deps dependencies) (datadogclient.Component, error) {
	client, err := createDatadogClient(deps.Config)
	if err != nil {
		return nil, err
	}
	dc := &datadogClient{
		datadogConfig: deps.Config,
		client:        client,
	}
	// Register a callback to refresh the client when the api_key or app_key changes
	deps.Config.OnUpdate(func(setting string, _, _ any) {
		if setting == "api_key" || setting == "app_key" {
			dc.refreshClient()
		}
	})
	return dc, nil
}

// GetStatus returns the status of the DatadogClient
func GetStatus(client datadogclient.Component) map[string]interface{} {
	status := make(map[string]interface{})

	switch ddCl := client.(type) {
	case *datadog.Client:
		// Can be nil if there's an error in NewDatadogClient()
		if ddCl == nil {
			return status
		}

		clientStatus := make(map[string]interface{})
		clientStatus["url"] = ddCl.GetBaseUrl()
		status["client"] = clientStatus
	case *datadogFallbackClient:
		if ddCl == nil {
			return status
		}

		status["lastUsedClient"] = ddCl.lastUsedClient
		clientsStatus := make([]map[string]interface{}, len(ddCl.clients))
		for i, individualClient := range ddCl.clients {
			clientsStatus[i] = make(map[string]interface{})
			clientsStatus[i]["url"] = individualClient.client.GetBaseUrl()
			clientsStatus[i]["lastQuerySucceeded"] = individualClient.lastQuerySucceeded
			if individualClient.lastFailure.IsZero() {
				clientsStatus[i]["lastFailure"] = "Never"
			} else {
				clientsStatus[i]["lastFailure"] = individualClient.lastFailure
			}
			if individualClient.lastSuccess.IsZero() {
				clientsStatus[i]["lastSuccess"] = "Never"
			} else {
				clientsStatus[i]["lastSuccess"] = individualClient.lastSuccess
			}
			if individualClient.lastFailure.IsZero() &&
				individualClient.lastSuccess.IsZero() {
				clientsStatus[i]["status"] = "Unknown"
			} else if individualClient.lastQuerySucceeded {
				clientsStatus[i]["status"] = "OK"
			} else {
				clientsStatus[i]["status"] = "Failed"
			}
			clientsStatus[i]["retryInterval"] = individualClient.retryInterval
		}
		status["clients"] = clientsStatus
	}

	return status
}
