// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// TODO investigate flaky unit tests on windows
//go:build !windows

package server

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/benbjohnson/clock"
	"github.com/cenkalti/backoff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer(t *testing.T) {
	t.Run("should accept payloads on any route", func(t *testing.T) {
		fi := NewServer(WithClock(clock.NewMock()))

		request, err := http.NewRequest(http.MethodPost, "/totoro", strings.NewReader("totoro|5|tag:valid,owner:pducolin"))
		assert.NoError(t, err, "Error creating POST request")
		response := httptest.NewRecorder()

		fi.handleDatadogRequest(response, request)

		assert.Equal(t, http.StatusOK, response.Code, "unexpected code")
	})

	t.Run("should accept GET requests on any other route", func(t *testing.T) {
		fi := NewServer(WithClock(clock.NewMock()))

		request, err := http.NewRequest(http.MethodGet, "/kiki", nil)
		assert.NoError(t, err, "Error creating GET request")
		response := httptest.NewRecorder()

		fi.handleDatadogRequest(response, request)

		assert.Equal(t, http.StatusOK, response.Code, "unexpected code")
	})

	t.Run("should accept GET requests on /fakeintake/payloads route", func(t *testing.T) {
		fi := NewServer(WithClock(clock.NewMock()))

		request, err := http.NewRequest(http.MethodGet, "/fakeintake/payloads?endpoint=/foo", nil)

		assert.NoError(t, err, "Error creating GET request")
		response := httptest.NewRecorder()

		fi.handleGetPayloads(response, request)
		assert.Equal(t, http.StatusOK, response.Code, "unexpected code")

		expectedResponse := api.APIFakeIntakePayloadsRawGETResponse{
			Payloads: []api.Payload{},
		}
		actualResponse := api.APIFakeIntakePayloadsRawGETResponse{}
		body, err := io.ReadAll(response.Body)
		assert.NoError(t, err, "Error reading response")
		assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
		json.Unmarshal(body, &actualResponse)

		assert.Equal(t, expectedResponse, actualResponse, "unexpected response")
	})

	t.Run("should not accept GET requests on /fakeintake/payloads route without endpoint query parameter", func(t *testing.T) {
		fi := NewServer(WithClock(clock.NewMock()))

		request, err := http.NewRequest(http.MethodGet, "/fakeintake/payloads", nil)

		assert.NoError(t, err, "Error creating GET request")
		response := httptest.NewRecorder()

		fi.handleGetPayloads(response, request)
		assert.Equal(t, http.StatusBadRequest, response.Code, "unexpected code")
		assert.Equal(t, "text/plain", response.Header().Get("Content-Type"))
	})

	t.Run("should store multiple payloads on any route and return them", func(t *testing.T) {
		clock := clock.NewMock()
		fi := NewServer(WithClock(clock))

		postSomeFakePayloads(t, fi)

		request, err := http.NewRequest(http.MethodGet, "/fakeintake/payloads?endpoint=/totoro", nil)
		assert.NoError(t, err, "Error creating GET request")
		getResponse := httptest.NewRecorder()
		fi.handleGetPayloads(getResponse, request)
		assert.Equal(t, http.StatusOK, getResponse.Code)
		assert.Equal(t, "application/json", getResponse.Header().Get("Content-Type"))
		actualGETResponse := api.APIFakeIntakePayloadsRawGETResponse{}
		body, err := io.ReadAll(getResponse.Body)
		assert.NoError(t, err, "Error reading GET response")
		err = json.Unmarshal(body, &actualGETResponse)
		assert.NoError(t, err, "Error parsing api.APIFakeIntakePayloadsRawGETResponse")
		expectedResponse := api.APIFakeIntakePayloadsRawGETResponse{
			Payloads: []api.Payload{
				{
					Timestamp: clock.Now().UTC(),
					Encoding:  "",
					Data:      []byte("totoro|7|tag:valid,owner:pducolin"),
				},
				{
					Timestamp: clock.Now().UTC(),
					Encoding:  "",
					Data:      []byte("totoro|5|tag:valid,owner:kiki"),
				},
			},
		}
		assert.Equal(t, expectedResponse, actualGETResponse)
	})

	t.Run("should store multiple payloads on any route and return them in json", func(t *testing.T) {
		clock := clock.NewMock()
		fi := NewServer(WithClock(clock))

		postSomeRealisticPayloads(t, fi)

		request, err := http.NewRequest(http.MethodGet, "/fakeintake/payloads?endpoint=/api/v2/logs&format=json", nil)
		assert.NoError(t, err, "Error creating GET request")
		getResponse := httptest.NewRecorder()

		fi.handleGetPayloads(getResponse, request)

		assert.Equal(t, http.StatusOK, getResponse.Code)
		expectedGETResponse := api.APIFakeIntakePayloadsJsonGETResponse{
			Payloads: []api.ParsedPayload{
				{
					Timestamp: clock.Now().UTC(),
					Data: []interface{}{map[string]interface{}{
						"hostname":  "totoro",
						"message":   "Hello, can you hear me",
						"service":   "callme",
						"source":    "Adele",
						"status":    "Info",
						"tags":      []interface{}{"singer:adele"},
						"timestamp": float64(0)}},
					Encoding: "gzip",
				},
			},
		}
		actualGETResponse := api.APIFakeIntakePayloadsJsonGETResponse{}
		body, err := io.ReadAll(getResponse.Body)
		assert.NoError(t, err, "Error reading GET response")
		json.Unmarshal(body, &actualGETResponse)
		assert.Equal(t, expectedGETResponse, actualGETResponse, "unexpected GET response")

	})

	t.Run("should accept GET requests on /fakeintake/health route", func(t *testing.T) {
		fi := NewServer(WithClock(clock.NewMock()))

		request, err := http.NewRequest(http.MethodGet, "/fakeintake/health", nil)

		assert.NoError(t, err, "Error creating GET request")
		response := httptest.NewRecorder()

		fi.handleFakeHealth(response, request)
		assert.Equal(t, http.StatusOK, response.Code, "unexpected code")
	})

	t.Run("should return error when calling stop on a non-started server", func(t *testing.T) {
		fi := NewServer(WithClock(clock.NewMock()))

		err := fi.Stop()
		assert.Error(t, err)
		assert.Equal(t, "server not running", err.Error())
	})

	t.Run("should correctly start a server with no ready channel defined", func(t *testing.T) {
		fi := NewServer(WithClock(clock.NewMock()))

		fi.Start()

		err := backoff.Retry(func() error {
			url := fi.URL()
			if url == "" {
				return errors.New("server not ready")
			}
			resp, err := http.Get(url + "/fakeintake/health")
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return errors.New("server not ready")
			}
			return nil
		}, backoff.WithMaxRetries(backoff.NewConstantBackOff(10*time.Millisecond), 25))
		require.NoError(t, err)
		err = fi.Stop()
		assert.NoError(t, err)
	})

	t.Run("should correctly notify when a server is ready", func(t *testing.T) {
		ready := make(chan bool, 1)
		fi := NewServer(WithClock(clock.NewMock()), WithReadyChannel(ready))
		fi.Start()
		ok := <-ready
		assert.True(t, ok)
		assert.NotEmpty(t, fi.URL())
		err := backoff.Retry(func() error {
			resp, err := http.Get(fi.URL() + "/fakeintake/health")
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return errors.New("server not ready")
			}
			return nil
		}, backoff.WithMaxRetries(backoff.NewConstantBackOff(10*time.Millisecond), 25))
		require.NoError(t, err)
		err = fi.Stop()
		assert.NoError(t, err)
	})
	t.Run("should store multiple payloads on any route and return the list of routes", func(t *testing.T) {
		fi := NewServer(WithClock(clock.NewMock()))

		postSomeFakePayloads(t, fi)

		request, err := http.NewRequest(http.MethodGet, "/fakeintake/routestats", nil)
		assert.NoError(t, err, "Error creating GET request")
		getResponse := httptest.NewRecorder()

		fi.handleGetRouteStats(getResponse, request)

		assert.Equal(t, http.StatusOK, getResponse.Code)

		expectedGETResponse := api.APIFakeIntakeRouteStatsGETResponse{
			Routes: map[string]api.RouteStat{
				"/totoro": {
					ID:    "/totoro",
					Count: 2,
				},
				"/kiki": {
					ID:    "/kiki",
					Count: 1,
				},
			},
		}
		actualGETResponse := api.APIFakeIntakeRouteStatsGETResponse{}
		body, err := io.ReadAll(getResponse.Body)
		assert.NoError(t, err, "Error reading GET response")
		json.Unmarshal(body, &actualGETResponse)

		assert.Equal(t, expectedGETResponse, actualGETResponse, "unexpected GET response")
	})

	t.Run("should handle flush requests", func(t *testing.T) {
		clock := clock.NewMock()
		fi := NewServer(WithClock(clock))

		postSomeFakePayloads(t, fi)

		request, err := http.NewRequest(http.MethodDelete, "/fakeintake/flushPayloads", nil)
		assert.NoError(t, err, "Error creating flush request")
		response := httptest.NewRecorder()

		fi.handleFlushPayloads(response, request)
		assert.Equal(t, http.StatusOK, response.Code, "unexpected code")
	})

	t.Run("should clean payloads older than 15 minutes", func(t *testing.T) {
		clock := clock.NewMock()
		fi := NewServer(WithClock(clock))
		fi.Start()

		postSomeFakePayloads(t, fi)

		request, err := http.NewRequest(http.MethodGet, "/fakeintake/payloads?endpoint=/totoro", nil)
		assert.NoError(t, err, "Error creating GET request")

		clock.Add(10 * time.Minute)

		response10Min := httptest.NewRecorder()
		var getResponse10Min api.APIFakeIntakePayloadsRawGETResponse

		fi.handleGetPayloads(response10Min, request)
		json.NewDecoder(response10Min.Body).Decode(&getResponse10Min)

		assert.Len(t, getResponse10Min.Payloads, 2, "should contain two elements before cleanup %+v", getResponse10Min)

		clock.Add(10 * time.Minute)

		response20Min := httptest.NewRecorder()
		var getResponse20Min api.APIFakeIntakePayloadsRawGETResponse

		fi.handleGetPayloads(response20Min, request)
		json.NewDecoder(response20Min.Body).Decode(&getResponse10Min)

		assert.Empty(t, getResponse20Min.Payloads, "should be empty after cleanup")
		fi.Stop()
	})

	t.Run("should clean payloads older than 15 minutes and keep recent payloads", func(t *testing.T) {
		clock := clock.NewMock()
		fi := NewServer(WithClock(clock))
		fi.Start()

		postSomeFakePayloads(t, fi)

		request, err := http.NewRequest(http.MethodGet, "/fakeintake/payloads?endpoint=/totoro", nil)
		assert.NoError(t, err, "Error creating GET request")

		clock.Add(10 * time.Minute)

		postSomeFakePayloads(t, fi)

		response10Min := httptest.NewRecorder()
		var getResponse10Min api.APIFakeIntakePayloadsRawGETResponse

		fi.handleGetPayloads(response10Min, request)
		json.NewDecoder(response10Min.Body).Decode(&getResponse10Min)

		assert.Len(t, getResponse10Min.Payloads, 4, "should contain 4 elements before cleanup")

		clock.Add(10 * time.Minute)

		response20Min := httptest.NewRecorder()
		var getResponse20Min api.APIFakeIntakePayloadsRawGETResponse

		fi.handleGetPayloads(response20Min, request)
		json.NewDecoder(response20Min.Body).Decode(&getResponse20Min)

		assert.Len(t, getResponse20Min.Payloads, 2, "should contain 2 elements after cleanup of only older elements")

		fi.Stop()
	})

	t.Run("should clean parsed payloads", func(t *testing.T) {
		clock := clock.NewMock()
		fi := NewServer(WithClock(clock))
		fi.Start()

		request, err := http.NewRequest(http.MethodGet, "/fakeintake/payloads?endpoint=/api/v2/logs&format=json", nil)
		assert.NoError(t, err, "Error creating GET request")

		postSomeRealisticPayloads(t, fi)

		clock.Add(10 * time.Minute)

		postSomeRealisticPayloads(t, fi)

		response10Min := httptest.NewRecorder()
		var getResponse10Min api.APIFakeIntakePayloadsJsonGETResponse

		fi.handleGetPayloads(response10Min, request)
		json.NewDecoder(response10Min.Body).Decode(&getResponse10Min)

		assert.Len(t, getResponse10Min.Payloads, 2, "should contain 2 elements before cleanup")

		clock.Add(10 * time.Minute)

		response20Min := httptest.NewRecorder()
		var getResponse20Min api.APIFakeIntakePayloadsJsonGETResponse

		fi.handleGetPayloads(response20Min, request)
		json.NewDecoder(response20Min.Body).Decode(&getResponse20Min)

		assert.Len(t, getResponse20Min.Payloads, 1, "should contain 1 elements after cleanup of only older elements")

		fi.Stop()
	})
}

func postSomeFakePayloads(t *testing.T, fi *Server) {
	request, err := http.NewRequest(http.MethodPost, "/totoro", strings.NewReader("totoro|7|tag:valid,owner:pducolin"))
	require.NoError(t, err, "Error creating POST request")
	postResponse := httptest.NewRecorder()
	fi.handleDatadogRequest(postResponse, request)

	request, err = http.NewRequest(http.MethodPost, "/totoro", strings.NewReader("totoro|5|tag:valid,owner:kiki"))
	require.NoError(t, err, "Error creating POST request")
	postResponse = httptest.NewRecorder()
	fi.handleDatadogRequest(postResponse, request)

	request, err = http.NewRequest(http.MethodPost, "/kiki", strings.NewReader("I am just a poor raw log"))
	require.NoError(t, err, "Error creating POST request")
	postResponse = httptest.NewRecorder()
	fi.handleDatadogRequest(postResponse, request)
}

//go:embed fixture_test/log_bytes
var logBytes []byte

func postSomeRealisticPayloads(t *testing.T, fi *Server) {
	request, err := http.NewRequest(http.MethodPost, "/api/v2/logs", bytes.NewBuffer(logBytes))
	require.NoError(t, err, "Error creating POST request")
	request.Header.Set("Content-Encoding", "gzip")
	postResponse := httptest.NewRecorder()
	fi.handleDatadogRequest(postResponse, request)
}
