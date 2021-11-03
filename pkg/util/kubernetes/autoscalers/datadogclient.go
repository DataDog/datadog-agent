// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

// +build kubeapiserver

package autoscalers

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"gopkg.in/zorkian/go-datadog-api.v2"
)

const (
	metricsEndpointPrefix = "https://api."
	metricsEndpointConfig = "external_metrics_provider.endpoint"
)

func NewDatadogClient() (DatadogClient, error) {
	if config.Datadog.IsSet("external_metrics_provider.endpoints") {
		var endpoints []config.Endpoint
		if err := config.Datadog.UnmarshalKey("external_metrics_provider.endpoints", &endpoints); err != nil {
			return nil, log.Errorf("could not parse external_metrics_provider.endpoints: %w", err)
		}

		return newDatadogFallbackClient(endpoints)
	}

	return newDatadogSingleClient()
}

// NewDatadogSingleClient generates a new client to query metrics from Datadog
func newDatadogSingleClient() (*datadog.Client, error) {
	apiKey := config.SanitizeAPIKey(config.Datadog.GetString("external_metrics_provider.api_key"))
	if apiKey == "" {
		apiKey = config.SanitizeAPIKey(config.Datadog.GetString("api_key"))
	}

	appKey := config.SanitizeAPIKey(config.Datadog.GetString("external_metrics_provider.app_key"))
	if appKey == "" {
		appKey = config.SanitizeAPIKey(config.Datadog.GetString("app_key"))
	}

	// DATADOG_HOST used to be the only way to set the external metrics
	// endpoint, so we need to keep backwards compatibility. In order of
	// priority, we use:
	//   - DD_EXTERNAL_METRICS_PROVIDER_ENDPOINT
	//   - DATADOG_HOST
	//   - DD_SITE
	endpoint := os.Getenv("DATADOG_HOST")
	if config.Datadog.GetString(metricsEndpointConfig) != "" || endpoint == "" {
		endpoint = config.GetMainEndpoint(metricsEndpointPrefix, metricsEndpointConfig)
	}

	if appKey == "" || apiKey == "" {
		return nil, errors.New("missing the api/app key pair to query Datadog")
	}

	log.Infof("Initialized the Datadog Client for HPA with endpoint %q", endpoint)

	client := datadog.NewClient(apiKey, appKey)
	client.HttpClient.Transport = httputils.CreateHTTPTransport()
	client.RetryTimeout = 3 * time.Second
	client.ExtraHeader["User-Agent"] = "Datadog-Cluster-Agent"
	client.SetBaseUrl(endpoint)

	return client, nil
}

// DatadogFallbackClient represents a datadog client able to query metrics to a second Datadog endpoint if the first one fails
type datadogFallbackClient struct {
	clients []datadog.Client
}

// NewDatadogFallbackClient generates a new client able to query metrics to a second Datadog endpoint if the first one fails
func newDatadogFallbackClient(endpoints []config.Endpoint) (*datadogFallbackClient, error) {
	if len(endpoints) == 0 {
		return nil, log.Errorf("external_metrics_provider.endpoints must be non-empty")
	}

	clients := []datadog.Client{}
	for _, endpoint := range endpoints {
		client := datadog.NewClient(endpoint.APIKey, endpoint.APPKey)
		client.HttpClient.Transport = httputils.CreateHTTPTransport()
		client.RetryTimeout = 3 * time.Second
		client.ExtraHeader["User-Agent"] = "Datadog-Cluster-Agent"
		client.SetBaseUrl(endpoint.URL)
		clients = append(clients, *client)
	}

	return &datadogFallbackClient{
		clients: clients,
	}, nil
}

func (cl *datadogFallbackClient) QueryMetrics(from, to int64, query string) ([]datadog.Series, error) {
	errs := errors.New("Failed to query metrics on all endpoints")
	for _, c := range cl.clients {
		if series, err := c.QueryMetrics(from, to, query); err != nil {
			log.Infof("Failed to query metrics on %s: %v", c.GetBaseUrl(), err)
			errs = fmt.Errorf("%w, Failed to query metrics on %s: %v", errs, c.GetBaseUrl(), err)
		} else {
			return series, nil
		}
	}
	return nil, errs
}

func (cl *datadogFallbackClient) GetRateLimitStats() map[string]datadog.RateLimit {
	return cl.clients[0].GetRateLimitStats()
}
