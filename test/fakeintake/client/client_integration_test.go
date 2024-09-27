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
			fmt.Sprintf("%s/totoro", fi.URL()),
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
