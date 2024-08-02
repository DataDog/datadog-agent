// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package datadogclientimpl

import (
	"errors"
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
		return nil, errors.New("missing the api/app key pair to query Datadog")
	}

	logger.Infof("Initialized the Datadog Client for HPA with endpoint %q", ddEndpoint)

	client := datadog.NewClient(apiKey, appKey)
	client.HttpClient.Transport = httputils.CreateHTTPTransport(cfg)
	client.RetryTimeout = 3 * time.Second
	client.ExtraHeader["User-Agent"] = "Datadog-Cluster-Agent"
	client.SetBaseUrl(ddEndpoint)

	return client, nil
}
