// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	_ "embed"

	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed fixtures/api_v2_series_response
var apiV2SeriesResponse []byte

//go:embed fixtures/api_v1_check_run_response
var apiV1CheckRunResponse []byte

//go:embed fixtures/api_v2_logs_response
var apiV2LogsResponse []byte

func TestClient(t *testing.T) {
	t.Run("getFakePayloads should properly format the request", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// allow requests only to "/foo/bar"
			routes := r.URL.Query()["endpoint"]

			payloads := []api.Payload{
				{
					Data: []byte(r.URL.Path),
				},
				{
					Data: []byte(fmt.Sprintf("%d", len(routes))),
				},
				{
					Data: []byte(routes[0]),
				},
			}
			// create fake response
			resp, err := json.Marshal(api.APIFakeIntakePayloadsGETResponse{
				Payloads: payloads,
			})
			require.NoError(t, err)
			w.Write(resp)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		payloads, err := client.getFakePayloads("/foo/bar")
		assert.NoError(t, err, "Error getting payloads")
		assert.Equal(t, 3, len(payloads))
		assert.Equal(t, "/fakeintake/payloads", string(payloads[0].Data))
		assert.Equal(t, "1", string(payloads[1].Data))
		assert.Equal(t, "/foo/bar", string(payloads[2].Data))
	})

	t.Run("getFakePayloads should handle response with errors", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		payloads, err := client.getFakePayloads("/foo/bar")
		assert.Error(t, err, "Expecting error")
		assert.Nil(t, payloads)
	})

	t.Run("getMetrics", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(apiV2SeriesResponse)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		err := client.getMetrics()
		assert.NoError(t, err)
		assert.True(t, client.metricAggregator.ContainsPayloadName("system.load.1"))
		assert.False(t, client.metricAggregator.ContainsPayloadName("totoro"))
		assert.True(t, client.metricAggregator.ContainsPayloadNameAndTags("snmp.ifAdminStatus", []string{"interface:lo", "snmp_profile:generic-router"}))
		assert.False(t, client.metricAggregator.ContainsPayloadNameAndTags("snmp.ifAdminStatus", []string{"totoro"}))
	})

	t.Run("GetMetric", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(apiV2SeriesResponse)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		metrics, err := client.GetMetric("snmp.ifAdminStatus")
		assert.NoError(t, err)
		assert.NotEmpty(t, aggregator.FilterByTags(metrics, []string{"interface:lo", "snmp_profile:generic-router"}))
		assert.Empty(t, aggregator.FilterByTags(metrics, []string{"totoro"}))
	})

	t.Run("FilterMetrics", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(apiV2SeriesResponse)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		metrics, err := client.FilterMetrics("snmp.sysUpTimeInstance",
			WithTags[*aggregator.MetricSeries]([]string{"snmp_device:172.25.0.3", "snmp_profile:generic-router"}),
			WithMetricValueHigherThan(4226040),
			WithMetricValueLowerThan(4226042),
			WithMetricValueInRange(4226000, 4226050))
		assert.NoError(t, err)
		assert.NotEmpty(t, metrics)
	})

	t.Run("getChekRun", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(apiV1CheckRunResponse)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		err := client.getCheckRuns()
		assert.NoError(t, err)
		assert.True(t, client.checkRunAggregator.ContainsPayloadName("snmp.can_check"))
		assert.False(t, client.checkRunAggregator.ContainsPayloadName("totoro"))
		assert.True(t, client.checkRunAggregator.ContainsPayloadNameAndTags("datadog.agent.check_status", []string{"check:snmp"}))
		assert.False(t, client.checkRunAggregator.ContainsPayloadNameAndTags("datadog.agent.check_status", []string{"totoro"}))
	})

	t.Run("GetCheckRun", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(apiV1CheckRunResponse)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		checks, err := client.GetCheckRun("datadog.agent.check_status")
		assert.NoError(t, err)
		assert.NotEmpty(t, aggregator.FilterByTags(checks, []string{"check:snmp"}))
		assert.Empty(t, aggregator.FilterByTags(checks, []string{"totoro"}))
	})

	t.Run("getLogs", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(apiV2LogsResponse)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		err := client.getLogs()
		assert.NoError(t, err)
		assert.True(t, client.logAggregator.ContainsPayloadName("testapp"))
		assert.False(t, client.logAggregator.ContainsPayloadName("totoro"))
	})

	t.Run("GetLog", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(apiV2LogsResponse)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		logs, err := client.GetLog("testapp")
		assert.NoError(t, err)
		assert.Equal(t, 2, len(logs))
		assert.Equal(t, "hello there, can you hear me", logs[0].Message)
		assert.Equal(t, "info", logs[0].Status)
		assert.Equal(t, "a new line of logs", logs[1].Message)
	})

	t.Run("FilterLogs", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(apiV2LogsResponse)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		logs, err := client.FilterLogs("testapp", WithMessageMatching(`^hello.*`), WithMessageContaining("hello there, can you hear"))
		assert.NoError(t, err)
		assert.Equal(t, 1, len(logs))
	})

	t.Run("GetServerHealth", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/fakeintake/health" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		client := NewClient(ts.URL)
		err := client.GetServerHealth()
		assert.NoError(t, err)
	})
}
