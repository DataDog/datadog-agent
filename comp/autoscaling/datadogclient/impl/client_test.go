// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogclientimpl

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"testing"
	"testing/synctest"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/zorkian/go-datadog-api.v2"
)

func TestNewSingleClient(t *testing.T) {
	cfg := config.NewMock(t)
	logger := logmock.New(t)
	cfg.Set("api_key", "apikey123", pkgconfigmodel.SourceLocalConfigProcess)
	cfg.Set("app_key", "appkey456", pkgconfigmodel.SourceLocalConfigProcess)
	datadogClient, err := createDatadogClient(cfg, logger)
	assert.NoError(t, err)
	dogCl, ok := datadogClient.(*datadog.Client)
	assert.True(t, ok)
	assert.False(t, dogCl == (*datadog.Client)(nil))
}

func TestNewFallbackClient(t *testing.T) {
	cfg := config.NewMock(t)
	logger := logmock.New(t)
	cfg.Set("api_key", "apikey123", pkgconfigmodel.SourceLocalConfigProcess)
	cfg.Set("app_key", "appkey456", pkgconfigmodel.SourceLocalConfigProcess)
	cfg.SetInTest(metricsRedundantEndpointConfig, []map[string]interface{}{{"site": "api.datadoghq.eu", "url": "https://api.datadoghq.eu", "api_key": "12345", "app_key": "67890"}})

	assert.True(t, cfg.IsConfigured(metricsRedundantEndpointConfig))
	datadogClient, err := createDatadogClient(cfg, logger)
	assert.NoError(t, err)
	fallbackCl, ok := datadogClient.(*datadogFallbackClient)
	assert.True(t, ok)
	assert.False(t, fallbackCl == (*datadogFallbackClient)(nil))
}

// go-datadog-api defaults to the process-wide http.DefaultClient, which has no timeout. Requests to
// Datadog must be bounded and the shared default client must stay untouched.
func TestClientsHaveRequestTimeout(t *testing.T) {
	defaultTransport, defaultTimeout := http.DefaultClient.Transport, http.DefaultClient.Timeout

	for _, tt := range []struct {
		name string
		// endpointTimeout is only set when setEndpointTimeout is true, otherwise the default is used.
		setEndpointTimeout bool
		endpointTimeout    int
		fallback           bool
		expected           time.Duration
	}{
		{name: "single client, default", expected: 30 * time.Second},
		{name: "single client, configured", setEndpointTimeout: true, endpointTimeout: 7, expected: 7 * time.Second},
		{name: "single client, disabled", setEndpointTimeout: true, endpointTimeout: 0, expected: 0},
		{name: "fallback client, default", fallback: true, expected: 30 * time.Second},
		{name: "fallback client, configured", fallback: true, setEndpointTimeout: true, endpointTimeout: 7, expected: 7 * time.Second},
		{name: "fallback client, disabled", fallback: true, setEndpointTimeout: true, endpointTimeout: 0, expected: 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.NewMock(t)
			cfg.Set("api_key", "apikey123", pkgconfigmodel.SourceLocalConfigProcess)
			cfg.Set("app_key", "appkey456", pkgconfigmodel.SourceLocalConfigProcess)
			if tt.setEndpointTimeout {
				cfg.Set(metricsEndpointTimeoutConfig, tt.endpointTimeout, pkgconfigmodel.SourceLocalConfigProcess)
			}
			if tt.fallback {
				cfg.SetInTest(metricsRedundantEndpointConfig, []map[string]interface{}{{"site": "api.datadoghq.eu", "url": "https://api.datadoghq.eu", "api_key": "12345", "app_key": "67890"}})
			}

			datadogClient, err := createDatadogClient(cfg, logmock.New(t))
			require.NoError(t, err)

			var clients []*datadog.Client
			if tt.fallback {
				fallbackCl, ok := datadogClient.(*datadogFallbackClient)
				require.True(t, ok)
				require.Len(t, fallbackCl.clients, 2)
				for _, c := range fallbackCl.clients {
					clients = append(clients, c.client)
				}
			} else {
				singleCl, ok := datadogClient.(*datadog.Client)
				require.True(t, ok)
				clients = append(clients, singleCl)
			}

			for _, client := range clients {
				assert.NotSame(t, http.DefaultClient, client.HttpClient, "must not share http.DefaultClient")
				assert.Equal(t, tt.expected, client.HttpClient.Timeout)
				assert.NotNil(t, client.HttpClient.Transport)
			}
		})
	}

	assert.Equal(t, defaultTransport, http.DefaultClient.Transport, "http.DefaultClient transport must not be modified")
	assert.Equal(t, defaultTimeout, http.DefaultClient.Timeout, "http.DefaultClient timeout must not be modified")
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// When no endpoint answers, the error of the fallback client must still be recognizable as the
// timeout that caused it, both when the endpoints are tried in order and when they are retried
// after a backoff.
func TestFallbackClientKeepsTimeoutErrors(t *testing.T) {
	// The retries of go-datadog-api back off with timers, which only take virtual time here.
	synctest.Test(t, func(t *testing.T) {
		newIndividualClient := func(baseURL string) *datadogIndividualClient {
			client := datadog.NewClient("api-key", "app-key")
			client.SetBaseUrl(baseURL)
			client.HttpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, context.DeadlineExceeded
			})}
			client.RetryTimeout = time.Millisecond
			return &datadogIndividualClient{client: client, lastQuerySucceeded: true, retryInterval: minRetryInterval}
		}
		fallbackCl := &datadogFallbackClient{
			clients: []*datadogIndividualClient{newIndividualClient("http://primary.test"), newIndividualClient("http://backup.test")},
			log:     logmock.New(t),
		}

		// Every endpoint is tried in order.
		_, err := fallbackCl.QueryMetrics(0, 1, "avg:foo{*}")
		require.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)

		// Both endpoints have just failed, so they are now skipped by the backoff policy and only
		// queried again because nothing else succeeded.
		_, err = fallbackCl.QueryMetrics(0, 1, "avg:foo{*}")
		require.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})
}

func TestExternalMetricsProviderEndpointAndRefresh(t *testing.T) {
	reqs := make(chan string, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)
		requestDump, _ := httputil.DumpRequest(r, true)
		reqs <- string(requestDump)
		w.WriteHeader(200)
		w.Write([]byte("{\"status\": \"ok\"}"))
	}))
	defer ts.Close()
	cfg := config.NewMock(t)
	logger := logmock.New(t)
	cfg.Set("api_key", "apikey123", pkgconfigmodel.SourceLocalConfigProcess)
	cfg.Set("app_key", "appkey456", pkgconfigmodel.SourceLocalConfigProcess)
	cfg.Set("external_metrics_provider.enabled", true, pkgconfigmodel.SourceLocalConfigProcess)
	cfg.SetInTest(metricsEndpointConfig, ts.URL)
	datadogClientProvides, err := NewComponent(Requires{Config: cfg, Log: logger})
	// test component creation
	datadogClientComp := datadogClientProvides.Comp
	assert.NoError(t, err)
	datadogClientWithRefresh, ok := datadogClientComp.(*datadogClientWrapper)
	assert.True(t, ok)
	assert.False(t, datadogClientWithRefresh == (*datadogClientWrapper)(nil))
	dogCl, ok := datadogClientWithRefresh.client.(*datadog.Client)
	assert.True(t, ok)
	assert.False(t, dogCl == (*datadog.Client)(nil))
	assert.Equal(t, dogCl.GetBaseUrl(), ts.URL) // "http://127.0.0.1:52118"

	// test query with component
	query := "This_is_a_test_query"
	datadogClientWithRefresh.QueryMetrics(0, 1, query)
	payload := <-reqs
	assert.Contains(t, payload, query)

	//refresh client
	newAPIKey := "fake_api_key"
	newAPPKey := "fake_app_key"
	cfg.Set("api_key", newAPIKey, pkgconfigmodel.SourceLocalConfigProcess)
	cfg.Set("app_key", newAPPKey, pkgconfigmodel.SourceLocalConfigProcess)
	assert.Eventually(t, func() bool {
		return datadogClientWithRefresh.numberOfRefreshes > 0
	}, 5*time.Second, 200*time.Millisecond)
	datadogClientWithRefresh.QueryMetrics(0, 1, query)
	payload2 := <-reqs
	assert.Contains(t, payload2, newAPIKey)
	assert.Contains(t, payload2, newAPPKey)

	// noop client
	cfg.Set("external_metrics_provider.enabled", false, pkgconfigmodel.SourceLocalConfigProcess)
	noopProvides, err := NewComponent(Requires{Config: cfg, Log: logger})
	assert.NoError(t, err)
	_, ok = noopProvides.Comp.(*ImplNone)
	assert.True(t, ok)
}
