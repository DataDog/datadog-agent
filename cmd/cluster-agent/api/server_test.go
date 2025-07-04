// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"fmt"
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
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("cluster_agent.auth_token", "abc123")
	util.InitDCAAuthToken(mockConfig)
	ipcComp := ipcmock.New(t)

	tests := []struct {
		path, authToken    string
		expectedStatusCode int
	}{
		{
			"/api/v1/metadata",
			"abc123",
			http.StatusForbidden,
		},
		{
			"/api/v1/metadata/node/namespace/pod",
			"abc123",
			http.StatusOK,
		},
		{
			"/api/v1/metadata/node/namespace/pod",
			"imposter",
			http.StatusForbidden,
		},
		{
			"/version",
			"abc123",
			http.StatusOK,
		},
		{
			"/api/v1/cluster/id",
			"abc123",
			http.StatusOK,
		},
		{
			"/version",
			"bandit!",
			http.StatusForbidden,
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("#%d", i), func(t *testing.T) {
			req, err := http.NewRequest("GET", tt.path, nil)
			require.NoError(t, err)

			req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", tt.authToken))

			rr := httptest.NewRecorder()

			nopHandler := func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}

			handler := validateToken(ipcComp)(http.HandlerFunc(nopHandler))

			handler.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatusCode, rr.Code)
		})
	}
}
