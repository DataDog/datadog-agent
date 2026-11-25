// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package datadogclientimpl

import (
	"errors"
	"net/http"
	"os"
	"time"

	"gopkg.in/zorkian/go-datadog-api.v2"

	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	logComp "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

// NewDatadogSingleClient generates a new client to query metrics from Datadog
// datadog.Client struct is defined in gopkg.in/zorkian/go-datadog-api.v2, provides apis of QueryMetrics and GetRateLimitStats
func newDatadogSingleClient(cfg configComponent.Component, logger logComp.Component) (*datadog.Client, error) {
	apiKey := utils.SanitizeAPIKey(cfg.GetString("external_metrics_provider.api_key"))
	if apiKey == "" {
		apiKey = utils.SanitizeAPIKey(cfg.GetString("api_key"))
	}

	appKey := utils.SanitizeAPIKey(cfg.GetString("external_metrics_provider.app_key"))
	if appKey == "" {
		appKey = utils.SanitizeAPIKey(cfg.GetString("app_key"))
	}

	// DATADOG_HOST used to be the only way to set the external metrics
	// endpoint, so we need to keep backwards compatibility. In order of
	// priority, we use:
	//   - DD_EXTERNAL_METRICS_PROVIDER_ENDPOINT
	//   - DATADOG_HOST
	//   - DD_SITE
	ddEndpoint := os.Getenv("DATADOG_HOST")
	if cfg.GetString(metricsEndpointConfig) != "" || ddEndpoint == "" {
		ddEndpoint = utils.GetMainEndpoint(cfg, metricsEndpointPrefix, metricsEndpointConfig)
	}

	if appKey == "" || apiKey == "" {
		logger.Warn("If metrics provider is enabled, Datadog application key must be set for read access to metrics.")
		return nil, errors.New("missing the api/app key pair to query Datadog")
	}

	logger.Infof("Initialized the Datadog Client for HPA with endpoint %q", ddEndpoint)

	return newDatadogAPIClient(cfg, apiKey, appKey, ddEndpoint), nil
}

// newDatadogAPIClient creates a go-datadog-api client for the given endpoint.
func newDatadogAPIClient(cfg configComponent.Component, apiKey, appKey, endpoint string) *datadog.Client {
	client := datadog.NewClient(apiKey, appKey)
	// datadog.NewClient defaults to the process-wide http.DefaultClient, use a dedicated one so that
	// the transport and the timeout are not applied to every other user of http.DefaultClient.
	client.HttpClient = &http.Client{
		Transport: httputils.CreateHTTPTransport(cfg),
		// go-datadog-api uses no timeout by default, so a request that never completes would
		// otherwise only be bounded by the OS (minutes). The timeout covers the whole exchange,
		// including reading the response. A value <= 0 disables it.
		Timeout: time.Duration(cfg.GetInt(metricsEndpointTimeoutConfig)) * time.Second,
	}
	client.RetryTimeout = 3 * time.Second
	client.ExtraHeader["User-Agent"] = "Datadog-Cluster-Agent"
	client.SetBaseUrl(endpoint)

	return client
}
