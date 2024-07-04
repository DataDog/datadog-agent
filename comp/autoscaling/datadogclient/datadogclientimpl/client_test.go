// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogclientimpl

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/stretchr/testify/assert"
	"gopkg.in/zorkian/go-datadog-api.v2"
)

func TestNewSingleClient(t *testing.T) {
	cfg := config.Mock(t)
	cfg.Set("api_key", "apikey123", pkgconfigmodel.SourceLocalConfigProcess)
	cfg.Set("app_key", "appkey456", pkgconfigmodel.SourceLocalConfigProcess)
	datadogClient, err := createDatadogClient(cfg)
	assert.NoError(t, err)
	dogCl, ok := datadogClient.(*datadog.Client)
	assert.True(t, ok)
	assert.False(t, dogCl == (*datadog.Client)(nil))
}

func TestNewFallbackClient(t *testing.T) {
	cfg := config.Mock(t)
	cfg.Set("api_key", "apikey123", pkgconfigmodel.SourceLocalConfigProcess)
	cfg.Set("app_key", "appkey456", pkgconfigmodel.SourceLocalConfigProcess)
	cfg.SetWithoutSource(metricsRedundantEndpointConfig,
		[]endpoint{
			{
				"api.datadoghq.eu",
				"https://api.datadoghq.eu",
				"12345",
				"67890",
			},
		})
	assert.True(t, cfg.IsSet(metricsRedundantEndpointConfig))
	datadogClient, err := createDatadogClient(cfg)
	assert.NoError(t, err)
	fallbackCl, ok := datadogClient.(*datadogFallbackClient)
	assert.True(t, ok)
	assert.False(t, fallbackCl == (*datadogFallbackClient)(nil))
}

func TestExternalMetricsProviderEndpoint(t *testing.T) {
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
	cfg := config.Mock(t)
	cfg.Set("api_key", "apikey123", pkgconfigmodel.SourceLocalConfigProcess)
	cfg.Set("app_key", "appkey456", pkgconfigmodel.SourceLocalConfigProcess)
	cfg.SetWithoutSource(metricsEndpointConfig, ts.URL)
	datadogClientComp, err := newDatadogClient(dependencies{Config: cfg})
	assert.NoError(t, err)
	datadogClientWithRefresh, ok := datadogClientComp.(*datadogClient)
	assert.True(t, ok)
	assert.False(t, datadogClientWithRefresh == (*datadogClient)(nil))
	dogCl, ok := datadogClientWithRefresh.client.(*datadog.Client)
	assert.True(t, ok)
	assert.False(t, dogCl == (*datadog.Client)(nil))
	assert.Equal(t, dogCl.GetBaseUrl(), ts.URL) // "http://127.0.0.1:52118"
	query := "This_is_a_test_query"
	dogCl.QueryMetrics(0, 1, query)
	payload := <-reqs
	assert.Contains(t, payload, query)

	//refresh client
	newAPIKey := "fake_api_key"
	newAPPKey := "fake_app_key"
	cfg.Set("api_key", newAPIKey, pkgconfigmodel.SourceLocalConfigProcess)
	cfg.Set("app_key", newAPPKey, pkgconfigmodel.SourceLocalConfigProcess)
	refreshedDogCl, ok := datadogClientWithRefresh.client.(*datadog.Client)
	assert.True(t, ok)
	assert.False(t, refreshedDogCl == (*datadog.Client)(nil))
	assert.NotEqual(t, dogCl, refreshedDogCl)
	refreshedDogCl.QueryMetrics(0, 1, query)
	payload2 := <-reqs
	assert.Contains(t, payload2, newAPIKey)
	assert.Contains(t, payload2, newAPPKey)
	log.Infof(payload2)

}
