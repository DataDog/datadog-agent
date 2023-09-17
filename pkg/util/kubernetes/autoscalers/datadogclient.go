// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build kubeapiserver

package autoscalers

import (
	"errors"
	"fmt"
	"os"
	"time"

	"gopkg.in/zorkian/go-datadog-api.v2"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	metricsEndpointPrefix = "https://api."
	metricsEndpointConfig = "external_metrics_provider.endpoint"
)

// NewDatadogClient configures and returns a new DatadogClient
func NewDatadogClient() (DatadogClient, error) {
	if config.Datadog.IsSet("external_metrics_provider.endpoints") {
		var endpoints []config.Endpoint
		if err := config.Datadog.UnmarshalKey("external_metrics_provider.endpoints", &endpoints); err != nil {
			return nil, log.Errorf("could not parse external_metrics_provider.endpoints: %v", err)
		}

		return newDatadogFallbackClient(endpoints)
	}

	return newDatadogSingleClient()
}

// NewDatadogSingleClient generates a new client to query metrics from Datadog
func newDatadogSingleClient() (*datadog.Client, error) {
	apiKey := configUtils.SanitizeAPIKey(config.Datadog.GetString("external_metrics_provider.api_key"))
	if apiKey == "" {
		apiKey = configUtils.SanitizeAPIKey(config.Datadog.GetString("api_key"))
	}

	appKey := configUtils.SanitizeAPIKey(config.Datadog.GetString("external_metrics_provider.app_key"))
	if appKey == "" {
		appKey = configUtils.SanitizeAPIKey(config.Datadog.GetString("app_key"))
	}

	// DATADOG_HOST used to be the only way to set the external metrics
	// endpoint, so we need to keep backwards compatibility. In order of
	// priority, we use:
	//   - DD_EXTERNAL_METRICS_PROVIDER_ENDPOINT
	//   - DATADOG_HOST
	//   - DD_SITE
	endpoint := os.Getenv("DATADOG_HOST")
	if config.Datadog.GetString(metricsEndpointConfig) != "" || endpoint == "" {
		endpoint = utils.GetMainEndpoint(config.Datadog, metricsEndpointPrefix, metricsEndpointConfig)
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

type datadogIndividualClient struct {
	client             *datadog.Client
	lastQuerySucceeded bool
	lastFailure        time.Time
	lastSuccess        time.Time
	retryInterval      time.Duration
}

const (
	minRetryInterval = 30 * time.Second
	maxRetryInterval = 30 * time.Minute
)

// DatadogFallbackClient represents a datadog client able to query metrics to a second Datadog endpoint if the first one fails
type datadogFallbackClient struct {
	clients        []*datadogIndividualClient
	lastUsedClient int
}

// NewDatadogFallbackClient generates a new client able to query metrics to a second Datadog endpoint if the first one fails
func newDatadogFallbackClient(endpoints []config.Endpoint) (*datadogFallbackClient, error) {
	if len(endpoints) == 0 {
		return nil, log.Errorf("external_metrics_provider.endpoints must be non-empty")
	}

	defaultClient, err := newDatadogSingleClient()
	if err != nil {
		return nil, err
	}

	ddFallbackClient := &datadogFallbackClient{
		clients: []*datadogIndividualClient{
			{
				client:             defaultClient,
				lastQuerySucceeded: true,
				retryInterval:      minRetryInterval,
			},
		},
	}
	for _, endpoint := range endpoints {
		client := datadog.NewClient(endpoint.APIKey, endpoint.APPKey)
		client.HttpClient.Transport = httputils.CreateHTTPTransport()
		client.RetryTimeout = 3 * time.Second
		client.ExtraHeader["User-Agent"] = "Datadog-Cluster-Agent"
		client.SetBaseUrl(endpoint.URL)
		ddFallbackClient.clients = append(
			ddFallbackClient.clients,
			&datadogIndividualClient{
				client:             client,
				lastQuerySucceeded: true,
				retryInterval:      minRetryInterval,
			})
	}

	return ddFallbackClient, nil
}

func (ic *datadogIndividualClient) queryMetrics(from, to int64, query string) ([]datadog.Series, error) {
	series, err := ic.client.QueryMetrics(from, to, query)
	if err == nil {
		ic.lastQuerySucceeded = true
		ic.lastSuccess = time.Now()
		ic.retryInterval /= 2
		if ic.retryInterval < minRetryInterval {
			ic.retryInterval = minRetryInterval
		}
		return series, err
	}

	ic.lastQuerySucceeded = false
	ic.lastFailure = time.Now()
	ic.retryInterval *= 2
	if ic.retryInterval > maxRetryInterval {
		ic.retryInterval = maxRetryInterval
	}
	return series, err
}

func (ic *datadogIndividualClient) hasFailedRecently() bool {
	if ic.lastQuerySucceeded {
		return false
	}

	return ic.lastFailure.Add(ic.retryInterval).After(time.Now())
}

func (cl *datadogFallbackClient) QueryMetrics(from, to int64, query string) ([]datadog.Series, error) {
	errs := errors.New("Failed to query metrics on all endpoints")

	skippedClients := []*datadogIndividualClient{}

	for i, c := range cl.clients {

		if c.hasFailedRecently() {
			skippedClients = append(skippedClients, c)
			continue
		}

		series, err := c.queryMetrics(from, to, query)
		if err == nil {
			if i != cl.lastUsedClient {
				log.Warnf("Switching external metrics source provider from %s to %s",
					cl.clients[cl.lastUsedClient].client.GetBaseUrl(),
					c.client.GetBaseUrl())
			}
			cl.lastUsedClient = i
			return series, nil
		}

		log.Infof("Failed to query metrics on %s: %s", c.client.GetBaseUrl(), err.Error())
		errs = fmt.Errorf("%w, Failed to query metrics on %s: %s", errs, c.client.GetBaseUrl(), err.Error())
	}

	for _, c := range skippedClients {
		log.Infof("Although we shouldn’t query %s because of the backoff policy, we’re going to do so because no other valid endpoint succeeded so far.", c.client.GetBaseUrl())
		series, err := c.queryMetrics(from, to, query)
		if err == nil {
			return series, nil
		}

		errs = fmt.Errorf("%w, Failed to query metrics on %s: %v", errs, c.client.GetBaseUrl(), err)
	}

	return nil, errs
}

func (cl *datadogFallbackClient) GetRateLimitStats() map[string]datadog.RateLimit {
	for _, c := range cl.clients {
		if c.lastQuerySucceeded {
			return c.client.GetRateLimitStats()
		}
	}
	return map[string]datadog.RateLimit{}
}

// GetStatus returns the status of the DatadogClient
func GetStatus(datadogClient DatadogClient) map[string]interface{} {
	status := make(map[string]interface{})

	switch ddCl := datadogClient.(type) {
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
