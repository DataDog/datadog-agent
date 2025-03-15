// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"

	"github.com/gorilla/mux"
)

func setupRoutes() *mux.Router {
	apiProviders := []api.EndpointProvider{
		api.NewAgentEndpointProvider(func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte("OK"))
		}, "/dynamic_route", "GET").Provider,
	}

	router := mux.NewRouter()
	SetupHandlers(
		router,
		apiProviders,
	)

	return router
}

func TestSetupHandlers(t *testing.T) {
	testcases := []struct {
		route    string
		method   string
		wantCode int
	}{
		{
			route:    "/dynamic_route",
			method:   "GET",
			wantCode: 200,
		},
	}
	router := setupRoutes()
	ts := httptest.NewServer(router)
	defer ts.Close()

	for _, tc := range testcases {
		req, err := http.NewRequest(tc.method, ts.URL+tc.route, nil)
		require.NoError(t, err)

		resp, err := ts.Client().Do(req)
		require.NoError(t, err)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		assert.Equal(t, tc.wantCode, resp.StatusCode, "%s %s failed with a %d, want %d", tc.method, tc.route, resp.StatusCode, tc.wantCode)
	}
}
