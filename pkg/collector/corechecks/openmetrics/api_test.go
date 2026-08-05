// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package openmetrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
)

func TestReusableScraperAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, err := w.Write([]byte(`
# TYPE app_up gauge
app_up 1
`))
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	scraper, err := NewScraperFromYAML([]byte(`
openmetrics_endpoint: `+server.URL+`
namespace: reusable
metrics:
  - app_up
`), "reusable:1")
	require.NoError(t, err)

	mockSender := mocksender.NewMockSender(t, checkid.ID("reusable:1"))
	mockSender.SetupAcceptAll()

	require.NoError(t, scraper.Scrape(mockSender))
	mockSender.AssertMetric(t, "Gauge", "reusable.app_up", 1, "", []string{"endpoint:" + server.URL})
}

func TestReusableScraperAPIUnsupportedConfig(t *testing.T) {
	_, err := NewScraperFromYAML([]byte(`
openmetrics_endpoint: http://127.0.0.1:1/metrics
metrics:
  - app_up
auth_type: digest
`), "reusable:1")

	require.Error(t, err)
	require.True(t, IsUnsupportedConfig(err))
}

func TestReusableScraperAPINilScraper(t *testing.T) {
	var scraper *Scraper
	require.ErrorContains(t, scraper.Scrape(mocksender.NewMockSender(t, checkid.ID("reusable:1"))), "openmetrics scraper is not configured")
}
