// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package datadogclientimpl implements the datadogclient component interface
package datadogclientimpl

import (
	"fmt"
	"sync"

	"gopkg.in/zorkian/go-datadog-api.v2"

	datadogclient "github.com/DataDog/datadog-agent/comp/autoscaling/datadogclient/def"
	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	logComp "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
)

// Requires defines the dependencies for the datadogclient component
type Requires struct {
	Config configComponent.Component
	Log    logComp.Component
}

// Provides defines the output of the datadogclient component
type Provides struct {
	Comp           datadogclient.Component
	StatusProvider status.InformationProvider
}

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

// NewComponent creates a new datadogclient component
func NewComponent(reqs Requires) (Provides, error) {
	defaultNoneComp := NewNone()
	provides := Provides{Comp: defaultNoneComp,
		StatusProvider: status.NewInformationProvider(statusProvider{dc: defaultNoneComp}),
	}
	if !reqs.Config.GetBool("external_metrics_provider.enabled") {
		return provides, nil
	}
	client, err := createDatadogClient(reqs.Config, reqs.Log)
	if err != nil {
		reqs.Log.Errorf("fail to create datadog client for metrics provider, error: %v", err)
		return provides, nil
	}
	dc := &datadogClientWrapper{
		datadogConfig:     reqs.Config,
		client:            client,
		numberOfRefreshes: 0,
		log:               reqs.Log,
	}
	// Register a callback to refresh the client when the api_key or app_key changes
	reqs.Config.OnUpdate(func(setting string, _, _ any, _ uint64) {
		if setting == "api_key" || setting == "app_key" {
			dc.refreshClient()
		}
	})
	provides.Comp = dc
	provides.StatusProvider = status.NewInformationProvider(statusProvider{
		dc: dc.client,
	})
	return provides, nil
}

// datadogClientWrapper is a wrapper around the datadog.Client, which allows for
// refresh of the client pointer in case of app/api key changes
type datadogClientWrapper struct {
	client            datadogclient.Component
	mux               sync.RWMutex
	datadogConfig     configComponent.Component
	log               logComp.Component
	numberOfRefreshes int
}

var _ datadogclient.Component = (*datadog.Client)(nil)       // client implemented by zorkian/go-datadog-api.v2
var _ datadogclient.Component = (*datadogClientWrapper)(nil) // ensure wrapper: datadogClientWrapper implements the interface

// QueryMetrics takes as input from, to (seconds from Unix Epoch) and query string and then requests
// timeseries data for that time peried
func (d *datadogClientWrapper) QueryMetrics(from, to int64, query string) ([]datadog.Series, error) {
	d.mux.RLock()
	defer d.mux.RUnlock()
	return d.client.QueryMetrics(from, to, query)
}

// GetRateLimitStats is a threadsafe getter to retrieve the rate limiting stats associated with the Client.
func (d *datadogClientWrapper) GetRateLimitStats() map[string]datadog.RateLimit {
	d.mux.RLock()
	defer d.mux.RUnlock()
	return d.client.GetRateLimitStats()
}

func (d *datadogClientWrapper) refreshClient() {
	newClient, err := createDatadogClient(d.datadogConfig, d.log)
	if err != nil {
		d.log.Errorf("error refreshing datadog client: %v", err)
		return
	}
	d.mux.Lock()
	defer d.mux.Unlock()
	d.client = newClient
	d.numberOfRefreshes++
	d.log.Infof("refreshed datadog client, number of refreshes: %d", d.numberOfRefreshes)
}

func createDatadogClient(cfg configComponent.Component, logger logComp.Component) (datadogclient.Component, error) {
	if cfg.IsSet(metricsRedundantEndpointConfig) {
		var endpoints []endpoint
		if err := structure.UnmarshalKey(cfg, metricsRedundantEndpointConfig, &endpoints); err != nil {
			return nil, fmt.Errorf("could not parse %s: %v", metricsRedundantEndpointConfig, err)
		}

		return newDatadogFallbackClient(cfg, logger, endpoints)
	}
	return newDatadogSingleClient(cfg, logger)
}

func (d *datadogClientWrapper) getNumberOfRefreshes() int {
	d.mux.RLock()
	defer d.mux.RUnlock()
	return d.numberOfRefreshes
}
