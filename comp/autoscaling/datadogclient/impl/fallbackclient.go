// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

// Package datadogclientimpl implements datadog client component for querying external metrics.
package datadogclientimpl

import (
	"errors"
	"fmt"
	"time"

	"gopkg.in/zorkian/go-datadog-api.v2"

	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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
func newDatadogFallbackClient(config configComponent.Component, endpoints []endpoint) (*datadogFallbackClient, error) {
	if len(endpoints) == 0 {
		return nil, log.Errorf("%s must be non-empty", metricsRedundantEndpointConfig)
	}

	defaultClient, err := newDatadogSingleClient(config)
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
	for _, e := range endpoints {
		client := datadog.NewClient(e.APIKey, e.APPKey)
		client.HttpClient.Transport = httputils.CreateHTTPTransport(config)
		client.RetryTimeout = 3 * time.Second
		client.ExtraHeader["User-Agent"] = "Datadog-Cluster-Agent"
		client.SetBaseUrl(e.URL)
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
