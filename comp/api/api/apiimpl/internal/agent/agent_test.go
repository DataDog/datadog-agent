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
		req, err := http.NewRequest(tc.method, ts.URL+tc.route, nil)
		require.NoError(t, err)

		resp, err := ts.Client().Do(req)
		require.NoError(t, err)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		assert.Equal(t, tc.wantCode, resp.StatusCode, "%s %s failed with a %d, want %d", tc.method, tc.route, resp.StatusCode, tc.wantCode)
	}
}

func TestInstallInfoAPIRoutes(t *testing.T) {
	// Instead of trying to set up a file, which isn't the purpose of this test, use environment variables
	originalTool := os.Getenv("DD_INSTALL_INFO_TOOL")
	originalToolVersion := os.Getenv("DD_INSTALL_INFO_TOOL_VERSION")
	originalInstallerVersion := os.Getenv("DD_INSTALL_INFO_INSTALLER_VERSION")

	os.Setenv("DD_INSTALL_INFO_TOOL", "test-tool")
	os.Setenv("DD_INSTALL_INFO_TOOL_VERSION", "1.0.0")
	os.Setenv("DD_INSTALL_INFO_INSTALLER_VERSION", "test-installer-1.0")

	// Cleanup function to restore original environment
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
	}{
		// Install info GET endpoint tests
		{
			name:         "install info get with GET method",
			route:        "/install-info/get",
			method:       "GET",
			payload:      nil,
			expectedCode: 200,
			description:  "GET request to install-info/get should succeed with test config",
		},
		{
			name:         "install info get with POST method",
			route:        "/install-info/get",
			method:       "POST",
			payload:      nil,
			expectedCode: 405,
			description:  "POST request to install-info/get should be rejected",
		},
		{
			name:         "install info get with PUT method",
			route:        "/install-info/get",
			method:       "PUT",
			payload:      nil,
			expectedCode: 405,
			description:  "PUT request to install-info/get should be rejected",
		},
		{
			name:         "install info get with DELETE method",
			route:        "/install-info/get",
			method:       "DELETE",
			payload:      nil,
			expectedCode: 405,
			description:  "DELETE request to install-info/get should be rejected",
		},

		// Install info SET endpoint tests
		{
			name:   "install info set with POST method and valid payload",
			route:  "/install-info/set",
			method: "POST",
			payload: installinfo.SetInstallInfoRequest{
				Tool:             "ECS",
				ToolVersion:      "1.0",
				InstallerVersion: "test-installer",
			},
			expectedCode: 200,
			description:  "POST request to install-info/set with valid payload should succeed",
		},
		{
			name:   "install info set with PUT method and valid payload",
			route:  "/install-info/set",
			method: "PUT",
			payload: installinfo.SetInstallInfoRequest{
				Tool:             "ECS",
				ToolVersion:      "1.0",
				InstallerVersion: "test-installer",
			},
			expectedCode: 200,
			description:  "PUT request to install-info/set with valid payload should succeed",
		},
		{
			name:         "install info set with GET method",
			route:        "/install-info/set",
			method:       "GET",
			payload:      nil,
			expectedCode: 405,
			description:  "GET request to install-info/set should be rejected",
		},
		{
			name:         "install info set with DELETE method",
			route:        "/install-info/set",
			method:       "DELETE",
			payload:      nil,
			expectedCode: 405,
			description:  "DELETE request to install-info/set should be rejected",
		},
		{
			name:         "install info set with POST method and empty payload",
			route:        "/install-info/set",
			method:       "POST",
			payload:      nil,
			expectedCode: 400,
			description:  "POST request to install-info/set with empty payload should return bad request",
		},
		{
			name:   "install info set with POST method and invalid JSON",
			route:  "/install-info/set",
			method: "POST",
			payload: map[string]interface{}{
				"invalid": "payload",
			},
			expectedCode: 400,
			description:  "POST request to install-info/set with invalid payload should return bad request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body []byte
			var err error

			if tt.payload != nil {
				body, err = json.Marshal(tt.payload)
				require.NoError(t, err, "Failed to marshal test payload")
			}

			req, err := http.NewRequest(tt.method, ts.URL+tt.route, bytes.NewReader(body))
			require.NoError(t, err, "Failed to create HTTP request")

			if tt.payload != nil {
				req.Header.Set("Content-Type", "application/json")
			}

			resp, err := ts.Client().Do(req)
			require.NoError(t, err, "Failed to execute HTTP request")
			defer resp.Body.Close()

			// Read and discard body to ensure connection reuse
			responseBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err, "Failed to read response body")

			assert.Equal(t, tt.expectedCode, resp.StatusCode,
				"%s: %s %s returned %d, expected %d. Response: %s",
				tt.description, tt.method, tt.route, resp.StatusCode, tt.expectedCode, string(responseBody))

			// Additional validation for successful responses
			if tt.expectedCode == 200 {
				// Verify Content-Type header is set for JSON responses
				contentType := resp.Header.Get("Content-Type")
				assert.Equal(t, "application/json", contentType, "Response should have JSON content type")

				// For GET operations, verify the response structure
				if tt.route == "/install-info/get" {
					var installInfo installinfo.InstallInfo
					err = json.Unmarshal(responseBody, &installInfo)
					assert.NoError(t, err, "Response should be valid InstallInfo JSON")

					assert.Equal(t, "test-tool", installInfo.Tool, "Tool should match test data")
					assert.Equal(t, "1.0.0", installInfo.ToolVersion, "ToolVersion should match test data")
					assert.Equal(t, "test-installer-1.0", installInfo.InstallerVersion, "InstallerVersion should match test data")
				}

				// For SET operations, verify the response structure
				if tt.route == "/install-info/set" {
					var response map[string]interface{}
					err = json.Unmarshal(responseBody, &response)
					assert.NoError(t, err, "Response should be valid JSON")

					success, exists := response["success"]
					assert.True(t, exists, "Response should contain 'success' field")
					assert.True(t, success.(bool), "Response should indicate success")

					message, exists := response["message"]
					assert.True(t, exists, "Response should contain 'message' field")
					assert.NotEmpty(t, message, "Response message should not be empty")
				}
			}

			// Additional validation for method not allowed responses
			if tt.expectedCode == 405 {
				// Gorilla Mux returns 405 for unsupported methods
				assert.Equal(t, 405, resp.StatusCode, "Should return Method Not Allowed for unsupported methods")
			}
		})
	}
}

func TestInstallInfoAPIRoutesWithRuntimeInfo(t *testing.T) {
	router := setupRoutes()
	ts := httptest.NewServer(router)
	defer ts.Close()

	setPayload := installinfo.SetInstallInfoRequest{
		Tool:             "ECS",
		ToolVersion:      "1.0",
		InstallerVersion: "test-installer",
	}

	body, err := json.Marshal(setPayload)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", ts.URL+"/install-info/set", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode, "Setting install info should succeed")

	req, err = http.NewRequest("GET", ts.URL+"/install-info/get", nil)
	require.NoError(t, err)

	resp, err = ts.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode, "Getting install info should succeed when runtime info is set")

	var installInfo installinfo.InstallInfo
	err = json.Unmarshal(responseBody, &installInfo)
	assert.NoError(t, err, "Response should be valid InstallInfo JSON")

	assert.Equal(t, "ECS", installInfo.Tool, "Tool should match what was set")
	assert.Equal(t, "1.0", installInfo.ToolVersion, "ToolVersion should match what was set")
	assert.Equal(t, "test-installer", installInfo.InstallerVersion, "InstallerVersion should match what was set")
}

func TestHandleSetInstallInfo(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		payload        interface{}
		expectedStatus int
		expectedError  bool
	}{
		{
			name:   "valid POST request",
			method: "POST",
			payload: installinfo.SetInstallInfoRequest{
				Tool:             "ECS",
				ToolVersion:      "1.0",
				InstallerVersion: "ecs-task-def-v1",
			},
			expectedStatus: http.StatusOK,
			expectedError:  false,
		},
		{
			name:           "invalid method",
			method:         "GET",
			payload:        nil,
			expectedStatus: http.StatusBadRequest,
			expectedError:  true,
		},
		{
			name:           "invalid JSON",
			method:         "POST",
			payload:        "invalid json",
			expectedStatus: http.StatusBadRequest,
			expectedError:  true,
		},
		{
			name:   "missing tool field",
			method: "POST",
			payload: installinfo.SetInstallInfoRequest{
				Tool:             "",
				ToolVersion:      "1.0",
				InstallerVersion: "ecs-task-def-v1",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  true,
		},
		{
			name:   "missing tool version field",
			method: "POST",
			payload: installinfo.SetInstallInfoRequest{
				Tool:             "ECS",
				ToolVersion:      "",
				InstallerVersion: "ecs-task-def-v1",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  true,
		},
		{
			name:   "missing installer version field",
			method: "POST",
			payload: installinfo.SetInstallInfoRequest{
				Tool:             "ECS",
				ToolVersion:      "1.0",
				InstallerVersion: "",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  true,
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

			req := httptest.NewRequest(tt.method, "/install-info/set", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			installinfo.HandleSetInstallInfo(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedError {
				var response installinfo.SetInstallInfoResponse
				err = json.NewDecoder(w.Body).Decode(&response)
				require.NoError(t, err)
				assert.False(t, response.Success)
				assert.NotEmpty(t, response.Message)
			} else {
				var response installinfo.SetInstallInfoResponse
				err = json.NewDecoder(w.Body).Decode(&response)
				require.NoError(t, err)
				assert.True(t, response.Success)
				assert.NotEmpty(t, response.Message)

				reqGet := httptest.NewRequest("GET", "/install-info/get", nil)
				installinfo.HandleGetInstallInfo(w, reqGet)
				var info installinfo.InstallInfo
				err = json.NewDecoder(w.Body).Decode(&info)
				require.NoError(t, err)
				assert.Equal(t, "ECS", info.Tool)
				assert.Equal(t, "1.0", info.ToolVersion)
				assert.Equal(t, "ecs-task-def-v1", info.InstallerVersion)
			}
		})
	}
}

func TestHandleGetInstallInfo(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		setupRuntime   bool
		expectedStatus int
	}{
		{
			name:           "valid GET request with runtime info",
			method:         "GET",
			setupRuntime:   true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "valid GET request without runtime info",
			method:         "GET",
			setupRuntime:   false,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "invalid method",
			method:         "POST",
			setupRuntime:   false,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupRuntime {
				info := &installinfo.InstallInfo{
					Tool:             "ECS",
					ToolVersion:      "1.0",
					InstallerVersion: "ecs-task-def-v1",
				}
				body, err := json.Marshal(installinfo.SetInstallInfoRequest{
					Tool:             info.Tool,
					ToolVersion:      info.ToolVersion,
					InstallerVersion: info.InstallerVersion,
				})
				require.NoError(t, err)
				w := httptest.NewRecorder()
				req := httptest.NewRequest("POST", "/install-info/set", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				installinfo.HandleSetInstallInfo(w, req)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/install-info/get", nil)

			installinfo.HandleGetInstallInfo(w, req)
			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				var info installinfo.InstallInfo
				err := json.NewDecoder(w.Body).Decode(&info)
				require.NoError(t, err)

				assert.Equal(t, "ECS", info.Tool)
				assert.Equal(t, "1.0", info.ToolVersion)
				assert.Equal(t, "ecs-task-def-v1", info.InstallerVersion)
			}
		})
	}
}
