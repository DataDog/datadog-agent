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
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

func TestServer(t *testing.T) {

	t.Run("should not run before start", func(t *testing.T) {
		fi := NewServer(WithClock(clock.NewMock()))
		assert.False(t, fi.IsRunning())
		assert.Empty(t, fi.URL())
	})

	t.Run("should return error when calling stop on a non-started server", func(t *testing.T) {
		fi := NewServer()
		err := fi.Stop()
		assert.Error(t, err)
		assert.Equal(t, "server not running", err.Error())
	})

	t.Run("should run after start", func(t *testing.T) {
		fi := NewServer(WithClock(clock.NewMock()), WithAddress("127.0.0.1:0"))
		fi.Start()
		defer fi.Stop()
		assert.EventuallyWithT(t, func(collect *assert.CollectT) {
			assert.True(collect, fi.IsRunning())
			assert.NotEmpty(collect, fi.URL())
			resp, err := http.Get(fi.URL() + "/fakeintake/health")
			assert.NoError(collect, err)
			if err != nil {
				return
			}
			defer resp.Body.Close()
			assert.Equal(collect, http.StatusOK, resp.StatusCode)
		}, 500*time.Millisecond, 10*time.Millisecond)
	})

	t.Run("should correctly notify when a server is ready", func(t *testing.T) {
		ready := make(chan bool, 1)
		fi := NewServer(WithClock(clock.NewMock()), WithReadyChannel(ready), WithAddress("127.0.0.1:0"))
		fi.Start()
		defer fi.Stop()
		ok := <-ready
		assert.True(t, ok)
		assert.NotEmpty(t, fi.URL())
		resp, err := http.Get(fi.URL() + "/fakeintake/health")
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("should accept payloads on any route", func(t *testing.T) {
		fi, _ := InitialiseForTests(t)
		defer fi.Stop()

		response, err := http.Post(fi.URL()+"/totoro", "text/plain", strings.NewReader("totoro|5|tag:valid,owner:pducolin"))
		require.NoError(t, err, "Error posting payload")
		defer response.Body.Close()
		assert.Equal(t, http.StatusOK, response.StatusCode, "unexpected code")
	})

	t.Run("should accept GET requests on any route", func(t *testing.T) {
		fi, _ := InitialiseForTests(t)
		defer fi.Stop()

		response, err := http.Get(fi.URL() + "/kiki")
		require.NoError(t, err, "Error on GET request")
		defer response.Body.Close()
		assert.Equal(t, http.StatusOK, response.StatusCode, "unexpected code")
	})

	t.Run("should accept GET requests on /fakeintake/payloads route", func(t *testing.T) {
		fi, _ := InitialiseForTests(t)
		defer fi.Stop()

		response, err := http.Get(fi.URL() + "/fakeintake/payloads?endpoint=/foo")
		require.NoError(t, err, "Error on GET request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode, "unexpected code")

		expectedResponse := api.APIFakeIntakePayloadsRawGETResponse{
			Payloads: []api.Payload{},
		}
		actualResponse := api.APIFakeIntakePayloadsRawGETResponse{}
		body, err := io.ReadAll(response.Body)
		assert.NoError(t, err, "Error reading response")
		assert.Equal(t, "application/json", response.Header.Get("Content-Type"))
		json.Unmarshal(body, &actualResponse)

		assert.Equal(t, expectedResponse, actualResponse, "unexpected response")
	})

	t.Run("should not accept GET requests on /fakeintake/payloads route without endpoint query parameter", func(t *testing.T) {
		fi, _ := InitialiseForTests(t)
		defer fi.Stop()

		response, err := http.Get(fi.URL() + "/fakeintake/payloads")
		require.NoError(t, err, "Error on GET request")
		defer response.Body.Close()
		assert.Equal(t, http.StatusBadRequest, response.StatusCode, "unexpected code")
		assert.Equal(t, "text/plain", response.Header.Get("Content-Type"))
	})

	t.Run("should store multiple payloads on any route and return them", func(t *testing.T) {
		fi, clock := InitialiseForTests(t)
		defer fi.Stop()

		PostSomeFakePayloads(t, fi.URL(), []TestTextPayload{
			{
				Endpoint: "/totoro",
				Data:     "totoro|7|tag:valid,owner:pducolin",
			},
			{
				Endpoint: "/totoro",
				Data:     "totoro|5|tag:valid,owner:kiki",
			},
			{
				Endpoint: "/kiki",
				Data:     "I am just a poor raw log",
			},
		})

		getResponse, err := http.Get(fi.URL() + "/fakeintake/payloads?endpoint=/totoro")
		require.NoError(t, err, "Error on GET request")
		defer getResponse.Body.Close()
		assert.Equal(t, http.StatusOK, getResponse.StatusCode, "unexpected code")
		assert.Equal(t, "application/json", getResponse.Header.Get("Content-Type"))
		actualGETResponse := api.APIFakeIntakePayloadsRawGETResponse{}
		body, err := io.ReadAll(getResponse.Body)
		assert.NoError(t, err, "Error reading GET response")
		err = json.Unmarshal(body, &actualGETResponse)
		assert.NoError(t, err, "Error parsing api.APIFakeIntakePayloadsRawGETResponse")
		expectedResponse := api.APIFakeIntakePayloadsRawGETResponse{
			Payloads: []api.Payload{
				{
					Timestamp: clock.Now().UTC(),
					Encoding:  "text/plain",
					Data:      []byte("totoro|7|tag:valid,owner:pducolin"),
				},
				{
					Timestamp: clock.Now().UTC(),
					Encoding:  "text/plain",
					Data:      []byte("totoro|5|tag:valid,owner:kiki"),
				},
			},
		}
		assert.Equal(t, expectedResponse, actualGETResponse)
	})

	t.Run("should store multiple payloads on any route and return them in json", func(t *testing.T) {
		fi, clock := InitialiseForTests(t)
		defer fi.Stop()

		PostSomeRealisticLogs(t, fi.URL())

		response, err := http.Get(fi.URL() + "/fakeintake/payloads?endpoint=/api/v2/logs&format=json")
		require.NoError(t, err, "Error creating GET request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode, "unexpected code")
		expectedGETResponse := api.APIFakeIntakePayloadsJsonGETResponse{
			Payloads: []api.ParsedPayload{
				{
					Timestamp: clock.Now().UTC(),
					Data: []interface{}{map[string]interface{}{
						"hostname":  "totoro",
						"message":   "Hello, can you hear me",
						"service":   "callme",
						"ddsource":  "Adele",
						"status":    "Info",
						"ddtags":    "singer:adele",
						"timestamp": float64(0)}},
					Encoding: "gzip",
				},
			},
		}
		actualGETResponse := api.APIFakeIntakePayloadsJsonGETResponse{}
		body, err := io.ReadAll(response.Body)
		assert.NoError(t, err, "Error reading GET response")
		json.Unmarshal(body, &actualGETResponse)
		assert.Equal(t, expectedGETResponse, actualGETResponse, "unexpected GET response")
	})

	t.Run("should store multiple payloads on any route and return the list of routes", func(t *testing.T) {
		fi, _ := InitialiseForTests(t)
		defer fi.Stop()

		PostSomeFakePayloads(t, fi.URL(), []TestTextPayload{
			{
				Endpoint: "/totoro",
				Data:     "totoro|7|tag:valid,owner:pducolin",
			},
			{
				Endpoint: "/totoro",
				Data:     "totoro|5|tag:valid,owner:kiki",
			},
			{
				Endpoint: "/kiki",
				Data:     "I am just a poor raw log",
			},
		})

		response, err := http.Get(fi.URL() + "/fakeintake/routestats")
		require.NoError(t, err, "Error on GET request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode, "unexpected code")

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
		body, err := io.ReadAll(response.Body)
		assert.NoError(t, err, "Error reading GET response")
		json.Unmarshal(body, &actualGETResponse)

		assert.Equal(t, expectedGETResponse, actualGETResponse, "unexpected GET response")
	})

	t.Run("should handle flush requests", func(t *testing.T) {
		fi, _ := InitialiseForTests(t)
		defer fi.Stop()

		httpClient := http.Client{}
		request, err := http.NewRequest(http.MethodDelete, fi.URL()+"/fakeintake/flushPayloads", nil)
		require.NoError(t, err, "Error creating flush request")
		response, err := httpClient.Do(request)
		require.NoError(t, err, "Error on flush request")
		defer response.Body.Close()
		assert.Equal(t, http.StatusOK, response.StatusCode, "unexpected code")
	})

	t.Run("should clean payloads older than 15 minutes", func(t *testing.T) {
		fi, clock := InitialiseForTests(t)
		defer fi.Stop()

		PostSomeFakePayloads(t, fi.URL(), []TestTextPayload{
			{
				Endpoint: "/totoro",
				Data:     "totoro|7|tag:valid,owner:pducolin",
			},
			{
				Endpoint: "/totoro",
				Data:     "totoro|5|tag:valid,owner:kiki",
			},
			{
				Endpoint: "/kiki",
				Data:     "I am just a poor raw log",
			},
		})

		clock.Add(10 * time.Minute)

		response10Min, err := http.Get(fi.URL() + "/fakeintake/payloads?endpoint=/totoro")
		require.NoError(t, err, "Error on GET request")
		defer response10Min.Body.Close()

		var getResponse10Min api.APIFakeIntakePayloadsRawGETResponse
		json.NewDecoder(response10Min.Body).Decode(&getResponse10Min)
		assert.Len(t, getResponse10Min.Payloads, 2, "should contain two elements before cleanup %+v", getResponse10Min)

		clock.Add(10 * time.Minute)

		response20Min, err := http.Get(fi.URL() + "/fakeintake/payloads?endpoint=/totoro")
		require.NoError(t, err, "Error on GET request")
		defer response20Min.Body.Close()
		var getResponse20Min api.APIFakeIntakePayloadsRawGETResponse
		json.NewDecoder(response20Min.Body).Decode(&getResponse20Min)
		assert.Empty(t, getResponse20Min.Payloads, "should be empty after cleanup")
	})

	t.Run("should clean payloads older than 15 minutes and keep recent payloads", func(t *testing.T) {
		fi, clock := InitialiseForTests(t)
		defer fi.Stop()

		PostSomeFakePayloads(t, fi.URL(), []TestTextPayload{
			{
				Endpoint: "/totoro",
				Data:     "totoro|7|tag:valid,owner:pducolin",
			},
			{
				Endpoint: "/totoro",
				Data:     "totoro|5|tag:valid,owner:kiki",
			},
			{
				Endpoint: "/kiki",
				Data:     "I am just a poor raw log",
			},
		})

		clock.Add(10 * time.Minute)

		PostSomeFakePayloads(t, fi.URL(), []TestTextPayload{
			{
				Endpoint: "/totoro",
				Data:     "totoro|7|tag:valid,owner:ponyo",
			},
			{
				Endpoint: "/totoro",
				Data:     "totoro|5|tag:valid,owner:mei",
			},
		})

		response10Min, err := http.Get(fi.URL() + "/fakeintake/payloads?endpoint=/totoro")
		require.NoError(t, err, "Error on GET request")
		defer response10Min.Body.Close()
		var getResponse10Min api.APIFakeIntakePayloadsRawGETResponse
		json.NewDecoder(response10Min.Body).Decode(&getResponse10Min)
		assert.Len(t, getResponse10Min.Payloads, 4, "should contain 4 elements before cleanup")

		clock.Add(10 * time.Minute)

		response20Min, err := http.Get(fi.URL() + "/fakeintake/payloads?endpoint=/totoro")
		require.NoError(t, err, "Error on GET request")
		defer response20Min.Body.Close()
		var getResponse20Min api.APIFakeIntakePayloadsRawGETResponse
		json.NewDecoder(response20Min.Body).Decode(&getResponse20Min)
		assert.Len(t, getResponse20Min.Payloads, 2, "should contain 2 elements after cleanup of only older elements")

		fi.Stop()
	})

	t.Run("should clean json parsed payloads", func(t *testing.T) {
		fi, clock := InitialiseForTests(t)
		defer fi.Stop()

		PostSomeRealisticLogs(t, fi.URL())

		clock.Add(10 * time.Minute)

		PostSomeRealisticLogs(t, fi.URL())

		response10Min, err := http.Get(fi.URL() + "/fakeintake/payloads?endpoint=/api/v2/logs&format=json")
		require.NoError(t, err, "Error on GET request")
		defer response10Min.Body.Close()
		var getResponse10Min api.APIFakeIntakePayloadsJsonGETResponse
		json.NewDecoder(response10Min.Body).Decode(&getResponse10Min)
		assert.Len(t, getResponse10Min.Payloads, 2, "should contain 2 elements before cleanup")

		clock.Add(10 * time.Minute)

		response20Min, err := http.Get(fi.URL() + "/fakeintake/payloads?endpoint=/api/v2/logs&format=json")
		require.NoError(t, err, "Error on GET request")
		defer response20Min.Body.Close()
		var getResponse20Min api.APIFakeIntakePayloadsJsonGETResponse
		json.NewDecoder(response20Min.Body).Decode(&getResponse20Min)
		assert.Len(t, getResponse20Min.Payloads, 1, "should contain 1 elements after cleanup of only older elements")
	})

	t.Run("should respond with custom response to /support/flare", func(t *testing.T) {
		fi, _ := InitialiseForTests(t)
		defer fi.Stop()

		response, err := http.Head(fi.URL() + "/support/flare")
		require.NoError(t, err, "Error on HEAD request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode, "unexpected code")
		assert.Equal(t, "application/json", response.Header.Get("Content-Type"))
		contentLength, err := strconv.Atoi(response.Header.Get("Content-Length"))
		require.NoError(t, err, "Error parsing Content-Length header")
		assert.Equal(t, 2, contentLength, "unexpected Content-Length")
		data, err := io.ReadAll(response.Body)
		require.NoError(t, err, "Error reading response body")
		assert.Empty(t, data, "unexpected HEAD response body")
	})

	t.Run("should accept POST response overrides", func(t *testing.T) {
		fi, _ := InitialiseForTests(t)
		defer fi.Stop()

		body := api.ResponseOverride{
			Method:      http.MethodPost,
			Endpoint:    "/totoro",
			StatusCode:  200,
			ContentType: "text/plain",
			Body:        []byte("catbus"),
		}
		data := new(bytes.Buffer)
		err := json.NewEncoder(data).Encode(body)
		require.NoError(t, err, "Error encoding request body")

		response, err := http.Post(fi.URL()+"/fakeintake/configure/override", "application/json", data)
		require.NoError(t, err, "Error creating POST request")
		defer response.Body.Close()
		assert.Equal(t, http.StatusOK, response.StatusCode, "unexpected code")
	})

	t.Run("should accept GET response overrides", func(t *testing.T) {
		fi, _ := InitialiseForTests(t)
		defer fi.Stop()

		body := api.ResponseOverride{
			Method:      http.MethodGet,
			Endpoint:    "/totoro",
			StatusCode:  200,
			ContentType: "text/plain",
			Body:        []byte("catbus"),
		}
		data := new(bytes.Buffer)
		err := json.NewEncoder(data).Encode(body)
		require.NoError(t, err, "Error encoding request body")
		response, err := http.Post(fi.URL()+"/fakeintake/configure/override", "application/json", data)
		require.NoError(t, err, "Error creating POST request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode, "unexpected code")
	})

	t.Run("should respond with overridden response for matching endpoint", func(t *testing.T) {
		fi, _ := InitialiseForTests(t)
		defer fi.Stop()

		body := api.ResponseOverride{
			Method:      http.MethodGet,
			Endpoint:    "/totoro",
			StatusCode:  200,
			ContentType: "text/plain",
			Body:        []byte("catbus"),
		}
		data := new(bytes.Buffer)
		err := json.NewEncoder(data).Encode(body)
		require.NoError(t, err, "Error encoding request body")
		response, err := http.Post(fi.URL()+"/fakeintake/configure/override", "application/json", data)
		require.NoError(t, err, "Error creating POST request")
		defer response.Body.Close()

		response, err = http.Get(fi.URL() + "/totoro")
		require.NoError(t, err, "Error on POST request")
		defer response.Body.Close()
		assert.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, "text/plain", response.Header.Get("Content-Type"))
		responseBody, err := io.ReadAll(response.Body)
		require.NoError(t, err, "Error reading response body")
		assert.Equal(t, "catbus", string(responseBody))
	})

	t.Run("should respond with overridden response for matching endpoint", func(t *testing.T) {
		fi, _ := InitialiseForTests(t)
		defer fi.Stop()

		body := api.ResponseOverride{
			Method:      http.MethodPost,
			Endpoint:    "/totoro",
			StatusCode:  200,
			ContentType: "text/plain",
			Body:        []byte("catbus"),
		}
		data := new(bytes.Buffer)
		err := json.NewEncoder(data).Encode(body)
		require.NoError(t, err, "Error encoding request body")
		response, err := http.Post(fi.URL()+"/fakeintake/configure/override", "application/json", data)
		require.NoError(t, err, "Error creating POST request")
		defer response.Body.Close()

		response, err = http.Post(fi.URL()+"/totoro", "text/plain", strings.NewReader("totoro|5|tag:valid,owner:pducolin"))
		require.NoError(t, err, "Error on POST request")
		defer response.Body.Close()
		assert.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, "text/plain", response.Header.Get("Content-Type"))
		responseBody, err := io.ReadAll(response.Body)
		require.NoError(t, err, "Error reading response body")
		assert.Equal(t, "catbus", string(responseBody))
	})

	t.Run("should respond with default response for non-matching endpoint", func(t *testing.T) {
		fi, _ := InitialiseForTests(t)
		defer fi.Stop()

		body := api.ResponseOverride{
			Method:      http.MethodPost,
			Endpoint:    "/totoro",
			StatusCode:  200,
			ContentType: "text/plain",
			Body:        []byte("catbus"),
		}
		data := new(bytes.Buffer)
		err := json.NewEncoder(data).Encode(body)
		require.NoError(t, err, "Error encoding request body")
		response, err := http.Post(fi.URL()+"/fakeintake/configure/override", "application/json", data)
		require.NoError(t, err, "Error creating POST request")
		defer response.Body.Close()

		response, err = http.Post(fi.URL()+"/kiki", "text/plain", strings.NewReader("kiki|4|tag:valid,owner:jiji"))
		require.NoError(t, err, "Error on POST request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, "application/json", response.Header.Get("Content-Type"))
		responseBody, err := io.ReadAll(response.Body)
		require.NoError(t, err, "Error reading response body")
		assert.Equal(t, []byte(`{"errors":[]}`), responseBody)
	})
}

type TestTextPayload struct {
	Endpoint string
	Data     string
}

// PostSomeFakePayloads posts some fake payloads to the given url
func PostSomeFakePayloads(t *testing.T, url string, payloads []TestTextPayload) {
	t.Helper()
	for _, payload := range payloads {
		url := url + payload.Endpoint
		response, err := http.Post(url, "text/plain", strings.NewReader(payload.Data))
		require.NoError(t, err, fmt.Sprintf("Error on POST request to url %s with data: %s", url, payload.Data))
		defer response.Body.Close()
	}
}

//go:embed fixtures/log_bytes
var logBytes []byte

func PostSomeRealisticLogs(t *testing.T, url string) {
	t.Helper()
	response, err := http.Post(url+"/api/v2/logs", "gzip", bytes.NewBuffer(logBytes))
	require.NoError(t, err, "Error on POST request")
	defer response.Body.Close()
}
