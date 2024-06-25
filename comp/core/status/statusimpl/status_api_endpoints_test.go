// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package statusimpl implements the status component interface
package statusimpl

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"go.uber.org/fx"

	"github.com/gorilla/mux"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/status"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func getTestComp(t *testing.T, withError bool) provides {
	deps := fxutil.Test[dependencies](t, fx.Options(
		config.MockModule(),
		logimpl.MockModule(),
		fx.Supply(
			agentParams,
			status.NewHeaderInformationProvider(mockHeaderProvider{
				data: map[string]interface{}{
					"header_key": "header_value",
				},
				name:  "headerMock",
				text:  "header_key: header_value\n",
				index: 0,
			}),
			status.NewInformationProvider(mockProvider{
				data: map[string]interface{}{
					"foo": "bar",
				},
				name:        "a",
				text:        " text from a\n",
				section:     "section",
				returnError: withError,
			}),
		),
	))

	return newStatus(deps)
}

func TestStatusAPIEndpoints(t *testing.T) {
	nowFunc = func() time.Time { return time.Unix(1515151515, 0) }
	startTimeProvider = time.Unix(1515151515, 0)
	originalTZ := os.Getenv("TZ")
	os.Setenv("TZ", "UTC")

	defer func() {
		nowFunc = time.Now
		startTimeProvider = pkgconfigsetup.StartTime
		os.Setenv("TZ", originalTZ)
	}()

	// Create a new instance of the statusImplementation struct
	provider := getTestComp(t, false)

	// Create a new instance of the statusImplementation struct with error
	providerWithError := getTestComp(t, true)

	tests := []struct {
		testDesc        string
		method          string
		routerPath      string
		testedPath      string
		httpHandler     http.HandlerFunc
		expectedBody    []byte
		expectedCode    int
		additionalTests func(t *testing.T, rr *httptest.ResponseRecorder)
	}{
		{
			testDesc:    "text format",
			method:      "GET",
			routerPath:  "/status",
			testedPath:  "/status?format=text",
			httpHandler: provider.APIGetStatus.Provider.HandlerFunc(),
			expectedBody: func() []byte {
				status, err := provider.Comp.GetStatus("text", false)
				require.NoError(t, err)
				return status
			}(),
			expectedCode: http.StatusOK,
			additionalTests: func(t *testing.T, rr *httptest.ResponseRecorder) {
				require.Equal(t, "text/plain", rr.Header().Get("Content-Type"))
			},
		},
		{
			testDesc:    "json format",
			method:      "GET",
			routerPath:  "/status",
			testedPath:  "/status?format=json",
			httpHandler: provider.APIGetStatus.Provider.HandlerFunc(),
			expectedBody: func() []byte {
				status, err := provider.Comp.GetStatus("json", false)
				require.NoError(t, err)
				return status
			}(),
			expectedCode: http.StatusOK,
			additionalTests: func(t *testing.T, rr *httptest.ResponseRecorder) {
				require.Equal(t, "application/json", rr.Header().Get("Content-Type"))
			},
		},
		{
			testDesc:    "unknown format",
			method:      "GET",
			routerPath:  "/status",
			testedPath:  "/status?format=unknown",
			httpHandler: provider.APIGetStatus.Provider.HandlerFunc(),
			expectedBody: func() []byte {
				status, err := provider.Comp.GetStatus("text", false)
				require.NoError(t, err)
				return status
			}(),
			expectedCode: http.StatusOK,
			additionalTests: func(t *testing.T, rr *httptest.ResponseRecorder) {
				require.Equal(t, "text/plain", rr.Header().Get("Content-Type"))
			},
		},
		{
			testDesc:    "with error",
			method:      "GET",
			routerPath:  "/status",
			testedPath:  "/status?format=text",
			httpHandler: providerWithError.APIGetStatus.Provider.HandlerFunc(),
			expectedBody: func() []byte {
				status, err := providerWithError.Comp.GetStatus("text", false)
				require.NoError(t, err)
				return status
			}(),
			expectedCode: http.StatusOK,
			additionalTests: func(t *testing.T, rr *httptest.ResponseRecorder) {
			},
		},
		{
			method:      "GET",
			routerPath:  "/sections",
			testedPath:  "/sections",
			httpHandler: provider.APIGetSectionList.Provider.HandlerFunc(),
			expectedBody: func() []byte {
				res, err := json.Marshal(provider.Comp.GetSections())
				require.NoError(t, err)
				return res
			}(),
			expectedCode: http.StatusOK,
			additionalTests: func(t *testing.T, rr *httptest.ResponseRecorder) {
				require.Equal(t, "application/json", rr.Header().Get("Content-Type"))
			},
		},
		{
			testDesc:    "with header section",
			method:      "GET",
			routerPath:  "/{component}/status",
			testedPath:  "/header/status",
			httpHandler: provider.APIGetSection.Provider.HandlerFunc(),
			expectedBody: func() []byte {
				status, err := provider.Comp.GetStatusBySections([]string{"header"}, "text", false)
				require.NoError(t, err)
				return status
			}(),
			expectedCode: http.StatusOK,
		},
		{
			testDesc:    "with unknown section text format",
			method:      "GET",
			routerPath:  "/{component}/status",
			testedPath:  "/unknown/status",
			httpHandler: provider.APIGetSection.Provider.HandlerFunc(),
			expectedBody: func() []byte {
				_, err := provider.Comp.GetStatusBySections([]string{"unknown"}, "text", false)
				require.Error(t, err)

				return []byte(fmt.Sprintf("Error getting status. Error: %v.\n", err))
			}(),
			expectedCode: http.StatusInternalServerError,
		},
		{
			testDesc:    "with unknown section json format",
			method:      "GET",
			routerPath:  "/{component}/status",
			testedPath:  "/unknown/status?format=json",
			httpHandler: provider.APIGetSection.Provider.HandlerFunc(),
			expectedBody: func() []byte {
				_, err := provider.Comp.GetStatusBySections([]string{"unknown"}, "json", false)
				require.Error(t, err)

				body, _ := json.Marshal(map[string]string{"error": fmt.Sprintf("Error getting status. Error: %v, Status: []", err)})
				return append(body, byte('\n')) // HTTP body contains empty newline at the end
			}(),
			expectedCode: http.StatusInternalServerError,
			additionalTests: func(t *testing.T, rr *httptest.ResponseRecorder) {
				require.Equal(t, "application/json", rr.Header().Get("Content-Type"))
			},
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s - %s [%s]", test.routerPath, test.method, test.testDesc), func(t *testing.T) {
			r := mux.NewRouter()
			r.HandleFunc(test.routerPath, test.httpHandler)

			// Create a new HTTP request
			req, err := http.NewRequest(test.method, test.testedPath, nil)
			require.NoError(t, err)

			// Create a new HTTP response recorder
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			// Check the response status code
			require.Equal(t, rr.Code, test.expectedCode)

			// Check the response content type
			require.Equal(t, test.expectedBody, rr.Body.Bytes())

			// Check additional tests
			if test.additionalTests != nil {
				test.additionalTests(t, rr)
			}
		})
	}

}
