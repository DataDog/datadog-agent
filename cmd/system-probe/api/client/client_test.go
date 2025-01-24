// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package client

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConstructURL(t *testing.T) {
	u := constructURL("", "/asdf?a=b")
	assert.Equal(t, "http://sysprobe/asdf?a=b", u)

	u = constructURL("zzzz", "/asdf?a=b")
	assert.Equal(t, "http://sysprobe/zzzz/asdf?a=b", u)

	u = constructURL("zzzz", "asdf")
	assert.Equal(t, "http://sysprobe/zzzz/asdf", u)
}

type expectedTelemetryValues struct {
	totalRequests      float64
	failedRequests     float64
	failedResponses    float64
	responseErrors     float64
	malformedResponses float64
}

func validateTelemetry(t *testing.T, module string, expected expectedTelemetryValues) {
	assert.Equal(t, expected.totalRequests, checkTelemetry.totalRequests.WithValues(module).Get(), "mismatched totalRequests counter value")
	assert.Equal(t, expected.failedRequests, checkTelemetry.failedRequests.WithValues(module).Get(), "mismatched failedRequest counter value")
	assert.Equal(t, expected.failedResponses, checkTelemetry.failedResponses.WithValues(module).Get(), "mismatched failedResponses counter value")
	assert.Equal(t, expected.responseErrors, checkTelemetry.responseErrors.WithValues(module).Get(), "mismatched responseErrors counter value")
	assert.Equal(t, expected.malformedResponses, checkTelemetry.malformedResponses.WithValues(module).Get(), "mismatched malformedResponses counter value")
}

func TestGetCheck(t *testing.T) {
	type testData struct {
		Str string
		Num int
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/test/check" {
			_, _ = w.Write([]byte(`{"Str": "asdf", "Num": 42}`))
		} else if r.URL.Path == "/malformed/check" {
			//this should fail in json.Unmarshal
			_, _ = w.Write([]byte("1"))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	client := &http.Client{Transport: &http.Transport{DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
		return net.Dial("tcp", server.Listener.Addr().String())
	}}}

	//test happy flow
	resp, err := GetCheck[testData](client, "test")
	require.NoError(t, err)
	assert.Equal(t, "asdf", resp.Str)
	assert.Equal(t, 42, resp.Num)
	validateTelemetry(t, "test", expectedTelemetryValues{1, 0, 0, 0, 0})

	//test responseError counter
	resp, err = GetCheck[testData](client, "foo")
	require.Error(t, err)
	validateTelemetry(t, "foo", expectedTelemetryValues{1, 0, 0, 1, 0})

	//test malformedResponses counter
	resp, err = GetCheck[testData](client, "malformed")
	require.Error(t, err)
	validateTelemetry(t, "malformed", expectedTelemetryValues{1, 0, 0, 0, 1})

	//test failedRequests counter
	server.Close()
	resp, err = GetCheck[testData](client, "test")
	require.Error(t, err)
	validateTelemetry(t, "test", expectedTelemetryValues{2, 1, 0, 0, 0})
}
