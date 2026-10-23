// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package containers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/cenkalti/backoff/v7"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/containers/benchmark"
)

const (
	// dddevSite is the site of dddev, the Datadog org the Agents of the e2e tests report to.
	dddevSite = "datadoghq.com"
	// dddevAppURL is the URL of the Datadog app of dddev.
	dddevAppURL = "https://dddev.datadoghq.com"
	// dddevQueryAttempts is the number of attempts of a query failing to reach the API. The
	// API client already retries the rate-limited (429) and server error (5xx) responses.
	dddevQueryAttempts = 3
)

// dddevClient queries the timeseries of dddev.
type dddevClient struct {
	metrics *datadogV1.MetricsApi
	keys    map[string]datadog.APIKey
}

// newDDDevClient creates a client authenticated with the API and application keys of the
// runner profile, those the Agents of the e2e tests report with.
func newDDDevClient() (*dddevClient, error) {
	apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	if err != nil {
		return nil, fmt.Errorf("unable to get the API key: %w", err)
	}
	appKey, err := runner.GetProfile().SecretStore().Get(parameters.APPKey)
	if err != nil {
		return nil, fmt.Errorf("unable to get the application key: %w", err)
	}

	cfg := datadog.NewConfiguration()
	cfg.HTTPClient = &http.Client{Timeout: time.Minute}
	cfg.RetryConfiguration.EnableRetry = true
	cfg.RetryConfiguration.MaxRetries = 5
	// Room for the 5 retries and their exponential backoff, and for the reset of a rate limit.
	cfg.RetryConfiguration.HTTPRetryTimeout = 5 * time.Minute

	return &dddevClient{
		metrics: datadogV1.NewMetricsApi(datadog.NewAPIClient(cfg)),
		keys: map[string]datadog.APIKey{
			"apiKeyAuth": {Key: apiKey},
			"appKeyAuth": {Key: appKey},
		},
	}, nil
}

// query returns the points of the single timeseries query returns over [from, to].
func (c *dddevClient) query(ctx context.Context, query string, from, to time.Time) ([]benchmark.Point, error) {
	series, err := c.querySeries(ctx, query, from, to)
	if err != nil {
		return nil, err
	}
	if len(series) != 1 {
		return nil, fmt.Errorf("query %q returned %d timeseries over [%s, %s], want 1", query, len(series), from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339))
	}
	return series[0], nil
}

// querySeries returns the points of each of the timeseries query returns over [from, to].
func (c *dddevClient) querySeries(ctx context.Context, query string, from, to time.Time) ([][]benchmark.Point, error) {
	ctx = context.WithValue(ctx, datadog.ContextAPIKeys, c.keys)
	ctx = context.WithValue(ctx, datadog.ContextServerVariables, map[string]string{"site": dddevSite})

	resp, err := backoff.Retry(ctx, func() (datadogV1.MetricsQueryResponse, error) {
		resp, httpResp, err := c.metrics.QueryMetrics(ctx, from.Unix(), to.Unix(), query)
		if httpResp != nil {
			_ = httpResp.Body.Close()
		}
		// An HTTP response means the API was reached: retryable statuses were already retried.
		if err != nil && httpResp != nil {
			return resp, backoff.Permanent(err)
		}
		return resp, err
	}, backoff.WithMaxTries(dddevQueryAttempts), backoff.WithBackOff(backoff.NewConstantBackOff(10*time.Second)))
	if err != nil {
		return nil, fmt.Errorf("query %q failed: %w", query, err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("query %q failed: %s", query, *resp.Error)
	}

	series := make([][]benchmark.Point, 0, len(resp.Series))
	for _, s := range resp.Series {
		var points []benchmark.Point
		for _, point := range s.Pointlist {
			if len(point) != 2 || point[0] == nil || point[1] == nil {
				continue
			}
			points = append(points, benchmark.Point{Timestamp: time.UnixMilli(int64(*point[0])), Value: *point[1]})
		}
		series = append(series, points)
	}
	return series, nil
}
