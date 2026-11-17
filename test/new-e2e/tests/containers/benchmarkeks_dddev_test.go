// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package containers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/cenkalti/backoff/v7"
	"github.com/samber/lo"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/containers/benchmark"
)

const (
	// dddevSite is the site of dddev, the Datadog org the Agents of the e2e tests report to.
	dddevSite = "datadoghq.com"
	// benchmarkDashboardID is the dashboard of dddev showing the data of a run of the benchmark.
	benchmarkDashboardID = "bkx-svx-pjc"
	// dddevAttempts is the number of attempts of a call failing to reach the API. The API
	// client already retries the rate-limited (429) and server error (5xx) responses.
	dddevAttempts = 3
)

// dddevAppURL is the URL of the Datadog app of dddev.
var dddevAppURL = url.URL{Scheme: "https", Host: "dddev." + dddevSite}

// dddevClient queries the timeseries of dddev, and posts events to it.
type dddevClient struct {
	metrics *datadogV1.MetricsApi
	events  *datadogV1.EventsApi
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

	apiClient := datadog.NewAPIClient(cfg)
	return &dddevClient{
		metrics: datadogV1.NewMetricsApi(apiClient),
		events:  datadogV1.NewEventsApi(apiClient),
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
	resp, err := callDDDev(ctx, c, func(ctx context.Context) (datadogV1.MetricsQueryResponse, *http.Response, error) {
		return c.metrics.QueryMetrics(ctx, from.Unix(), to.Unix(), query)
	})
	if err != nil {
		return nil, fmt.Errorf("query %q failed: %w", query, err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("query %q failed: %s", query, *resp.Error)
	}

	return lo.Map(resp.Series, func(s datadogV1.MetricsQueryMetadata, _ int) []benchmark.Point {
		return lo.FilterMap(s.Pointlist, func(point []*float64, _ int) (benchmark.Point, bool) {
			if len(point) != 2 || point[0] == nil || point[1] == nil {
				return benchmark.Point{}, false
			}
			return benchmark.Point{Timestamp: time.UnixMilli(int64(*point[0])), Value: *point[1]}, true
		})
	}), nil
}

// postEvent posts event. An event retried after a network error may be posted twice.
func (c *dddevClient) postEvent(ctx context.Context, event datadogV1.EventCreateRequest) error {
	if _, err := callDDDev(ctx, c, func(ctx context.Context) (datadogV1.EventCreateResponse, *http.Response, error) {
		return c.events.CreateEvent(ctx, event)
	}); err != nil {
		return fmt.Errorf("posting the event %q failed: %w", event.Title, err)
	}
	return nil
}

// callDDDev returns the response of call to the API of dddev, with the keys of c, retried up to
// dddevAttempts times while it fails to reach the API.
func callDDDev[T any](ctx context.Context, c *dddevClient, call func(context.Context) (T, *http.Response, error)) (T, error) {
	ctx = context.WithValue(ctx, datadog.ContextAPIKeys, c.keys)
	ctx = context.WithValue(ctx, datadog.ContextServerVariables, map[string]string{"site": dddevSite})
	return backoff.Retry(ctx, func() (T, error) {
		resp, httpResp, err := call(ctx)
		if httpResp != nil {
			_ = httpResp.Body.Close()
		}
		// An HTTP response means the API was reached: retryable statuses were already retried.
		if err != nil && httpResp != nil {
			return resp, backoff.Permanent(err)
		}
		return resp, err
	}, backoff.WithMaxTries(dddevAttempts), backoff.WithBackOff(backoff.NewConstantBackOff(10*time.Second)))
}
