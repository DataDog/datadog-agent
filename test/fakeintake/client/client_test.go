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

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed fixtures/api_v2_series_response
var apiV2SeriesResponse []byte

//go:embed fixtures/api_v1_check_run_response
var apiV1CheckRunResponse []byte

func TestClient(t *testing.T) {
	t.Run("getFakePayloads should properly format the request", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// allow requests only to "/foo/bar"
			routes := r.URL.Query()["endpoint"]

			payloads := [][]byte{}

			payloads = append(payloads, []byte(r.URL.Path))
			payloads = append(payloads, []byte(fmt.Sprintf("%d", len(routes))))
			payloads = append(payloads, []byte(routes[0]))
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
		assert.Equal(t, "/fakeintake/payloads", string(payloads[0]))
		assert.Equal(t, "1", string(payloads[1]))
		assert.Equal(t, "/foo/bar", string(payloads[2]))
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
