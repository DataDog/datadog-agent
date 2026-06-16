// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// TODO investigate flaky unit tests on windows
//go:build !windows

package client

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator/testutil"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/DataDog/datadog-agent/test/fakeintake/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegrationClient(t *testing.T) {
	t.Run("should get empty payloads from a server", func(t *testing.T) {
		fi, _ := server.InitialiseForTests(t)
		defer fi.Stop()

		client := NewClient(fi.URL())
		stats, err := client.RouteStats()
		require.NoError(t, err, "Failed waiting for fakeintake")
		assert.Empty(t, stats)
	})

	t.Run("should get all available payloads from a server on a given endpoint", func(t *testing.T) {
		fi, _ := server.InitialiseForTests(t)
		defer fi.Stop()

		// post a test payloads to fakeintake
		testEndpoint := "/foo/bar"
		resp, err := http.Post(fmt.Sprintf("%s%s", fi.URL(), testEndpoint), "text/plain", strings.NewReader("totoro|5|tag:valid,owner:pducolin"))
		assert.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		client := NewClient(fi.URL())

		stats, err := client.RouteStats()
		require.NoError(t, err, "Error getting payloads")
		expectedStats := map[string]int{
			"/foo/bar": 1,
		}
		assert.Equal(t, expectedStats, stats)
	})

	t.Run("should flush payloads from a server on flush request", func(t *testing.T) {
		fi, _ := server.InitialiseForTests(t)
		defer fi.Stop()

		// post a test payloads to fakeintake
		resp, err := http.Post(fmt.Sprintf("%s%s", fi.URL(), "/foo/bar"), "text/plain", strings.NewReader("totoro|5|tag:before,owner:pducolin"))
		assert.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		client := NewClient(fi.URL())

		stats, err := client.RouteStats()
		require.NoError(t, err, "Error getting payloads")
		expectedStats := map[string]int{
			"/foo/bar": 1,
		}
		assert.Equal(t, expectedStats, stats)

		// flush
		err = client.FlushServerAndResetAggregators()
		require.NoError(t, err, "Error flushing")

		// post another payload
		resp, err = http.Post(fmt.Sprintf("%s%s", fi.URL(), "/bar/foo"), "text/plain", strings.NewReader("ponyo|7|tag:after,owner:pducolin"))
		assert.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		stats, err = client.RouteStats()
		require.NoError(t, err, "Error getting payloads")
		// should return only the second payload
		expectedStats = map[string]int{
			"/bar/foo": 1,
		}
		assert.Equal(t, expectedStats, stats)
	})

	t.Run("should track v2 and v3 series endpoints separately in route stats", func(t *testing.T) {
		fi, _ := server.InitialiseForTests(t)
		defer fi.Stop()

		client := NewClient(fi.URL())

		// POST to the v2 series endpoint (any body is enough to register the route).
		resp, err := http.Post(fi.URL()+"/api/v2/series", "application/octet-stream", strings.NewReader("{}"))
		require.NoError(t, err)
		defer resp.Body.Close()

		// POST to the v3 series endpoint.
		resp, err = http.Post(fi.URL()+"/api/intake/metrics/v3/series", "application/x-protobuf", bytes.NewReader(nil))
		require.NoError(t, err)
		defer resp.Body.Close()

		stats, err := client.RouteStats()
		require.NoError(t, err)
		assert.Equal(t, map[string]int{
			"/api/v2/series":                1,
			"/api/intake/metrics/v3/series": 1,
		}, stats)
	})

	t.Run("FilterMetrics should find a metric posted to the v3 series endpoint", func(t *testing.T) {
		fi, _ := server.InitialiseForTests(t)
		defer fi.Stop()

		client := NewClient(fi.URL())

		payload := testutil.MinimalV3GaugePayload()
		resp, err := http.Post(fi.URL()+"/api/intake/metrics/v3/series", "application/x-protobuf", bytes.NewReader(payload))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		series, err := client.FilterMetrics("test.gauge")
		require.NoError(t, err)
		require.NotEmpty(t, series, "FilterMetrics must find a metric posted to the v3 endpoint")
		assert.Equal(t, "test.gauge", series[0].Metric)
		assert.Equal(t, []string{"env:test"}, series[0].Tags)

		// Confirm the payload landed on the v3 route and not v2.
		stats, err := client.RouteStats()
		require.NoError(t, err)
		assert.Equal(t, 1, stats["/api/intake/metrics/v3/series"])
		assert.Zero(t, stats["/api/v2/series"])
	})

	t.Run("FilterMetrics should merge results from v2 and v3 endpoints", func(t *testing.T) {
		fi, _ := server.InitialiseForTests(t)
		defer fi.Stop()

		client := NewClient(fi.URL())

		// POST a v3 payload carrying "test.gauge".
		v3payload := testutil.MinimalV3GaugePayload()
		resp, err := http.Post(fi.URL()+"/api/intake/metrics/v3/series", "application/x-protobuf", bytes.NewReader(v3payload))
		require.NoError(t, err)
		defer resp.Body.Close()

		// POST an empty v2 payload so both routes are recorded.
		resp, err = http.Post(fi.URL()+"/api/v2/series", "application/octet-stream", strings.NewReader("{}"))
		require.NoError(t, err)
		defer resp.Body.Close()

		// FilterMetrics reads from both endpoints; "test.gauge" was only on v3.
		series, err := client.FilterMetrics("test.gauge")
		require.NoError(t, err)
		assert.NotEmpty(t, series, "FilterMetrics must surface metrics from the v3 endpoint")

		stats, err := client.RouteStats()
		require.NoError(t, err)
		assert.Equal(t, 1, stats["/api/intake/metrics/v3/series"])
		assert.Equal(t, 1, stats["/api/v2/series"])
	})

	t.Run("should receive overridden response when configured on server", func(t *testing.T) {
		fi, _ := server.InitialiseForTests(t)
		defer fi.Stop()

		client := NewClient(fi.URL())
		err := client.ConfigureOverride(api.ResponseOverride{
			Method:      http.MethodPost,
			Endpoint:    "/totoro",
			StatusCode:  200,
			ContentType: "text/plain",
			Body:        []byte("catbus"),
		})
		require.NoError(t, err, "failed to configure override")

		t.Log("post a test payload to fakeintake and check that the override is applied")
		resp, err := http.Post(
			fi.URL()+"/totoro",
			"text/plain",
			strings.NewReader("totoro|5|tag:valid,owner:mei"),
		)
		require.NoError(t, err, "failed to post test payload")

		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "text/plain", resp.Header.Get("Content-Type"))

		buf := new(bytes.Buffer)
		n, err := buf.ReadFrom(resp.Body)
		require.NoError(t, err, "failed to read response body")
		assert.Equal(t, len("catbus"), int(n))
		assert.Equal(t, []byte("catbus"), buf.Bytes())
	})
}
