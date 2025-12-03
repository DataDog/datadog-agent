// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package modules

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	tracerouteutil "github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseParams(t *testing.T) {
	tests := []struct {
		name           string
		host           string
		params         map[string]string
		expectedConfig tracerouteutil.Config
		expectedError  string
	}{
		{
			name:   "only host",
			host:   "1.2.3.4",
			params: map[string]string{},
			expectedConfig: tracerouteutil.Config{
				DestHostname: "1.2.3.4",
			},
		},
		{
			name: "all config",
			host: "1.2.3.4",
			params: map[string]string{
				"port":               "42",
				"max_ttl":            "35",
				"timeout":            "1000",
				"traceroute_queries": "3",
				"e2e_queries":        "50",
			},
			expectedConfig: tracerouteutil.Config{
				DestHostname:      "1.2.3.4",
				DestPort:          42,
				MaxTTL:            35,
				Timeout:           1000,
				TracerouteQueries: 3,
				E2eQueries:        50,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			req, err := http.NewRequestWithContext(context.Background(), "GET", "http://example.com", nil)
			q := req.URL.Query()
			for k, v := range tt.params {
				q.Add(k, v)
			}
			req.URL.RawQuery = q.Encode()
			req = mux.SetURLVars(req, map[string]string{"host": tt.host})

			require.NoError(t, err)
			config, err := parseParams(req)
			assert.Equal(t, tt.expectedConfig, config)
			if tt.expectedError != "" {
				assert.EqualError(t, err, tt.expectedError)
			}
		})
	}
}

func TestHandleTracerouteReqError(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		errString      string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "bad request error",
			statusCode:     http.StatusBadRequest,
			errString:      "invalid parameter: port",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "invalid parameter: port",
		},
		{
			name:           "internal server error",
			statusCode:     http.StatusInternalServerError,
			errString:      "unable to run traceroute for host: example.com: connection timeout",
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "unable to run traceroute for host: example.com: connection timeout",
		},
		{
			name:           "not found error",
			statusCode:     http.StatusNotFound,
			errString:      "resource not found",
			expectedStatus: http.StatusNotFound,
			expectedBody:   "resource not found",
		},
		{
			name:           "empty error string",
			statusCode:     http.StatusInternalServerError,
			errString:      "",
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "",
		},
		{
			name:           "long error message",
			statusCode:     http.StatusBadRequest,
			errString:      "this is a very long error message that contains a lot of details about what went wrong during the traceroute operation",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "this is a very long error message that contains a lot of details about what went wrong during the traceroute operation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a response recorder to capture the response
			recorder := httptest.NewRecorder()

			// Call the function
			handleTracerouteReqError(recorder, tt.statusCode, tt.errString)

			// Assert the status code
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			// Assert the response body
			assert.Equal(t, tt.expectedBody, recorder.Body.String())
		})
	}
}
