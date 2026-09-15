// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestValidateTokenMiddleware(t *testing.T) {
	const dcaToken = "abc123"

	mockConfig := configmock.New(t)
	mockConfig.SetInTest("cluster_agent.auth_token", dcaToken)
	util.InitDCAAuthToken(mockConfig)
	ipcComp := ipcmock.New(t)
	localToken := ipcComp.GetAuthToken()

	tests := []struct {
		name string
		// method defaults to GET when empty.
		method    string
		path      string
		authToken string
		// authHeader, when set, is sent verbatim instead of a bearer authToken.
		authHeader string
		// omitAuthHeader sends no Authorization header at all.
		omitAuthHeader     bool
		expectedStatusCode int
		// expectHandlerRun guards against the wrapped handler executing its side
		// effects even though the middleware rejected the request.
		expectHandlerRun bool
	}{
		// External paths are called by Node Agents and authenticate with the
		// cluster-wide DCA token.
		{
			name:               "external path with DCA token",
			path:               "/api/v1/metadata/node/namespace/pod",
			authToken:          dcaToken,
			expectedStatusCode: http.StatusOK,
			expectHandlerRun:   true,
		},
		{
			name:               "external path with invalid token",
			path:               "/api/v1/metadata/node/namespace/pod",
			authToken:          "imposter",
			expectedStatusCode: http.StatusForbidden,
			expectHandlerRun:   false,
		},
		{
			name:               "version with DCA token",
			path:               "/version",
			authToken:          dcaToken,
			expectedStatusCode: http.StatusOK,
			expectHandlerRun:   true,
		},
		{
			name:               "version with invalid token",
			path:               "/version",
			authToken:          "bandit!",
			expectedStatusCode: http.StatusForbidden,
			expectHandlerRun:   false,
		},
		{
			name:               "cluster id with DCA token",
			path:               "/api/v1/cluster/id",
			authToken:          dcaToken,
			expectedStatusCode: http.StatusOK,
			expectHandlerRun:   true,
		},
		{
			name:               "node info with DCA token",
			path:               "/api/v1/info/node/some-node",
			authToken:          dcaToken,
			expectedStatusCode: http.StatusOK,
			expectHandlerRun:   true,
		},
		{
			name:               "node annotations with query string and DCA token",
			path:               "/api/v1/annotations/node/some-node?filter=kubelet",
			authToken:          dcaToken,
			expectedStatusCode: http.StatusOK,
			expectHandlerRun:   true,
		},
		{
			name:               "external path rejects the local token",
			path:               "/version",
			authToken:          localToken,
			expectedStatusCode: http.StatusForbidden,
			expectHandlerRun:   false,
		},

		// Local/admin paths must only ever accept the local IPC token. The DCA
		// token is a cluster-wide shared secret mounted into every Node Agent
		// pod, so it must not reach these handlers (VULN-84904).
		{
			name:               "admin stop rejects the DCA token",
			path:               "/stop",
			authToken:          dcaToken,
			expectedStatusCode: http.StatusForbidden,
			expectHandlerRun:   false,
		},
		{
			name:               "admin config rejects the DCA token",
			path:               "/config",
			authToken:          dcaToken,
			expectedStatusCode: http.StatusForbidden,
			expectHandlerRun:   false,
		},
		{
			name:               "admin flare rejects the DCA token",
			path:               "/flare",
			authToken:          dcaToken,
			expectedStatusCode: http.StatusForbidden,
			expectHandlerRun:   false,
		},
		{
			name:               "all-metadata rejects the DCA token",
			path:               "/api/v1/metadata",
			authToken:          dcaToken,
			expectedStatusCode: http.StatusForbidden,
			expectHandlerRun:   false,
		},
		{
			name:               "clusterchecks rebalance rejects the DCA token",
			path:               "/api/v1/clusterchecks/rebalance",
			authToken:          dcaToken,
			expectedStatusCode: http.StatusForbidden,
			expectHandlerRun:   false,
		},
		{
			name:               "admin stop accepts the local token",
			path:               "/stop",
			authToken:          localToken,
			expectedStatusCode: http.StatusOK,
			expectHandlerRun:   true,
		},
		{
			name:               "all-metadata accepts the local token",
			path:               "/api/v1/metadata",
			authToken:          localToken,
			expectedStatusCode: http.StatusOK,
			expectHandlerRun:   true,
		},
		{
			name:               "admin path with invalid token",
			path:               "/stop",
			authToken:          "imposter",
			expectedStatusCode: http.StatusForbidden,
			expectHandlerRun:   false,
		},

		// A slash inside the query string must not be counted as a path segment.
		// isExternalPath gates on the number of slash-separated segments, so an
		// admin path one segment short of an external rule could otherwise be
		// padded until it matched, downgrading it to DCA-token authentication
		// while the router still dispatched the admin handler.
		{
			name:               "admin rebalance rejects a DCA token padded with a query-string slash",
			method:             "POST",
			path:               "/api/v1/clusterchecks/rebalance?x=/y",
			authToken:          dcaToken,
			expectedStatusCode: http.StatusForbidden,
			expectHandlerRun:   false,
		},
		{
			name:               "all-endpointschecks rejects a DCA token padded with a query-string slash",
			path:               "/api/v1/endpointschecks/configs?x=/y",
			authToken:          dcaToken,
			expectedStatusCode: http.StatusForbidden,
			expectHandlerRun:   false,
		},
		{
			name:               "external path still accepts the DCA token with a slash in the query",
			path:               "/api/v1/annotations/node/some-node?filter=a/b",
			authToken:          dcaToken,
			expectedStatusCode: http.StatusOK,
			expectHandlerRun:   true,
		},

		// A missing or malformed Authorization header is a 401, not a 403.
		{
			name:               "missing authorization header",
			path:               "/stop",
			omitAuthHeader:     true,
			expectedStatusCode: http.StatusUnauthorized,
			expectHandlerRun:   false,
		},
		{
			name:               "unsupported authorization scheme",
			path:               "/stop",
			authHeader:         "Basic dXNlcjpwYXNzd29yZA==",
			expectedStatusCode: http.StatusUnauthorized,
			expectHandlerRun:   false,
		},
		{
			name:               "bearer with no token",
			path:               "/stop",
			authHeader:         "Bearer",
			expectedStatusCode: http.StatusForbidden,
			expectHandlerRun:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method := tt.method
			if method == "" {
				method = "GET"
			}

			req, err := http.NewRequest(method, tt.path, nil)
			require.NoError(t, err)

			switch {
			case tt.omitAuthHeader:
			case tt.authHeader != "":
				req.Header.Add("Authorization", tt.authHeader)
			default:
				req.Header.Add("Authorization", "Bearer "+tt.authToken)
			}

			rr := httptest.NewRecorder()

			handlerRun := false
			handler := validateToken(ipcComp)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				handlerRun = true
				w.WriteHeader(http.StatusOK)
			}))

			handler.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatusCode, rr.Code)
			assert.Equal(t, tt.expectHandlerRun, handlerRun,
				"wrapped handler execution did not match expectation")
		})
	}
}
