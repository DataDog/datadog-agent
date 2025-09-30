// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/pkg/util/installinfo"

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
		fullURL, err := url.JoinPath(ts.URL, tc.route)
		require.NoError(t, err)
		req, err := http.NewRequest(tc.method, fullURL, nil)
		require.NoError(t, err)

		resp, err := ts.Client().Do(req)
		require.NoError(t, err)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		assert.Equal(t, tc.wantCode, resp.StatusCode, "%s %s failed with a %d, want %d", tc.method, tc.route, resp.StatusCode, tc.wantCode)
	}
}

func TestInstallInfoAPIRoutes(t *testing.T) {
	// Backup original environment
	originalTool := os.Getenv("DD_INSTALL_INFO_TOOL")
	originalToolVersion := os.Getenv("DD_INSTALL_INFO_TOOL_VERSION")
	originalInstallerVersion := os.Getenv("DD_INSTALL_INFO_INSTALLER_VERSION")

	os.Setenv("DD_INSTALL_INFO_TOOL", "test-tool")
	os.Setenv("DD_INSTALL_INFO_TOOL_VERSION", "1.0.0")
	os.Setenv("DD_INSTALL_INFO_INSTALLER_VERSION", "test-installer-1.0")

	defer func() {
		if originalTool == "" {
			os.Unsetenv("DD_INSTALL_INFO_TOOL")
		} else {
			os.Setenv("DD_INSTALL_INFO_TOOL", originalTool)
		}
		if originalToolVersion == "" {
			os.Unsetenv("DD_INSTALL_INFO_TOOL_VERSION")
		} else {
			os.Setenv("DD_INSTALL_INFO_TOOL_VERSION", originalToolVersion)
		}
		if originalInstallerVersion == "" {
			os.Unsetenv("DD_INSTALL_INFO_INSTALLER_VERSION")
		} else {
			os.Setenv("DD_INSTALL_INFO_INSTALLER_VERSION", originalInstallerVersion)
		}
	}()

	router := setupRoutes()
	ts := httptest.NewServer(router)
	defer ts.Close()

	tests := []struct {
		name         string
		route        string
		method       string
		payload      interface{}
		expectedCode int
		description  string
		assertFunc   func(t *testing.T, resp *http.Response, body []byte)
	}{
		{
			name:         "install info get with GET method",
			route:        "/install-info",
			method:       "GET",
			payload:      nil,
			expectedCode: 200,
			description:  "GET request to install-info should succeed with test config",
			assertFunc: func(t *testing.T, resp *http.Response, body []byte) {
				assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
				var installInfo installinfo.InstallInfo
				err := json.Unmarshal(body, &installInfo)
				assert.NoError(t, err)
				assert.Equal(t, "test-tool", installInfo.Tool)
				assert.Equal(t, "1.0.0", installInfo.ToolVersion)
				assert.Equal(t, "test-installer-1.0", installInfo.InstallerVersion)
			},
		},
		{
			name:         "install info get with DELETE method",
			route:        "/install-info",
			method:       "DELETE",
			payload:      nil,
			expectedCode: 405,
			description:  "DELETE request to install-info should be rejected",
		},
		{
			name:   "install info set with POST method and valid payload",
			route:  "/install-info",
			method: "POST",
			payload: installinfo.SetInstallInfoRequest{
				Tool:             "ECS",
				ToolVersion:      "1.0",
				InstallerVersion: "test-installer",
			},
			expectedCode: 200,
			description:  "POST request to install-info with valid payload should succeed",
			assertFunc: func(t *testing.T, resp *http.Response, body []byte) {
				assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
				var response map[string]interface{}
				err := json.Unmarshal(body, &response)
				assert.NoError(t, err)
				assert.True(t, response["success"].(bool))
				assert.NotEmpty(t, response["message"])

				getURL, err := url.JoinPath(ts.URL, "/install-info")
				require.NoError(t, err)
				getReq, err := http.NewRequest("GET", getURL, nil)
				require.NoError(t, err)

				getResp, err := ts.Client().Do(getReq)
				require.NoError(t, err)
				defer getResp.Body.Close()

				var info installinfo.InstallInfo
				err = json.NewDecoder(getResp.Body).Decode(&info)
				require.NoError(t, err)
				assert.Equal(t, "ECS", info.Tool)
				assert.Equal(t, "1.0", info.ToolVersion)
				assert.Equal(t, "test-installer", info.InstallerVersion)
			},
		},
		{
			name:   "install info set with PUT method and valid payload",
			route:  "/install-info",
			method: "PUT",
			payload: installinfo.SetInstallInfoRequest{
				Tool:             "ECS",
				ToolVersion:      "1.0",
				InstallerVersion: "test-installer",
			},
			expectedCode: 200,
			description:  "PUT request to install-info with valid payload should succeed",
			assertFunc: func(t *testing.T, resp *http.Response, body []byte) {
				assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
				var response map[string]interface{}
				err := json.Unmarshal(body, &response)
				assert.NoError(t, err)
				assert.True(t, response["success"].(bool))
				assert.NotEmpty(t, response["message"])

				getURL, err := url.JoinPath(ts.URL, "/install-info")
				require.NoError(t, err)
				getReq, err := http.NewRequest("GET", getURL, nil)
				require.NoError(t, err)

				getResp, err := ts.Client().Do(getReq)
				require.NoError(t, err)
				defer getResp.Body.Close()

				var info installinfo.InstallInfo
				err = json.NewDecoder(getResp.Body).Decode(&info)
				require.NoError(t, err)
				assert.Equal(t, "ECS", info.Tool)
				assert.Equal(t, "1.0", info.ToolVersion)
				assert.Equal(t, "test-installer", info.InstallerVersion)
			},
		},
		{
			name:         "install info set with POST method and empty payload",
			route:        "/install-info",
			method:       "POST",
			payload:      nil,
			expectedCode: 400,
			description:  "POST request to install-info with empty payload should return bad request",
		},
		{
			name:   "install info set with POST method and invalid JSON",
			route:  "/install-info",
			method: "POST",
			payload: map[string]interface{}{
				"invalid": "payload",
			},
			expectedCode: 400,
			description:  "POST request to install-info with invalid payload should return bad request",
		},
		{
			name:   "install info set with POST method and missing tool field",
			route:  "/install-info",
			method: "POST",
			payload: installinfo.SetInstallInfoRequest{
				Tool:             "",
				ToolVersion:      "1.0",
				InstallerVersion: "ecs-task-def-v1",
			},
			expectedCode: 400,
			description:  "POST request to install-info with missing tool field should return bad request",
		},
		{
			name:   "install info set with POST method and missing tool version field",
			route:  "/install-info",
			method: "POST",
			payload: installinfo.SetInstallInfoRequest{
				Tool:             "ECS",
				ToolVersion:      "",
				InstallerVersion: "ecs-task-def-v1",
			},
			expectedCode: 400,
			description:  "POST request to install-info with missing tool version field should return bad request",
		},
		{
			name:   "install info set with POST method and missing installer version field",
			route:  "/install-info",
			method: "POST",
			payload: installinfo.SetInstallInfoRequest{
				Tool:             "ECS",
				ToolVersion:      "1.0",
				InstallerVersion: "",
			},
			expectedCode: 400,
			description:  "POST request to install-info with missing installer version field should return bad request",
		},
		{
			name:         "install info set with POST method and invalid JSON string",
			route:        "/install-info",
			method:       "POST",
			payload:      "invalid json",
			expectedCode: 400,
			description:  "POST request to install-info with invalid JSON string should return bad request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body []byte
			var err error

			if tt.payload != nil {
				if str, ok := tt.payload.(string); ok {
					body = []byte(str)
				} else {
					body, err = json.Marshal(tt.payload)
					require.NoError(t, err)
				}
			}

			fullURL, err := url.JoinPath(ts.URL, tt.route)
			require.NoError(t, err)
			req, err := http.NewRequest(tt.method, fullURL, bytes.NewReader(body))
			require.NoError(t, err)

			if tt.payload != nil {
				req.Header.Set("Content-Type", "application/json")
			}

			resp, err := ts.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			responseBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedCode, resp.StatusCode,
				"%s: %s %s returned %d, expected %d. Response: %s",
				tt.description, tt.method, tt.route, resp.StatusCode, tt.expectedCode, string(responseBody))

			if tt.assertFunc != nil {
				tt.assertFunc(t, resp, responseBody)
			}
		})
	}
}
