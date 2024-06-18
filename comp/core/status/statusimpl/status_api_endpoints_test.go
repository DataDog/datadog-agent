// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package statusimpl implements the status component interface
package statusimpl

import (
	"encoding/json"
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

func getTestComp(t *testing.T) provides {
	deps := fxutil.Test[dependencies](t, fx.Options(
		config.MockModule(),
		logimpl.MockModule(),
		fx.Supply(
			agentParams,
			status.NewInformationProvider(mockProvider{
				data: map[string]interface{}{
					"foo": "bar",
				},
				name:    "a",
				text:    " text from a\n",
				section: "section",
			}),
		),
	))

	return newStatus(deps)
}

func TestStatusImplementation_GetStatus(t *testing.T) {
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
	provider := getTestComp(t)

	tests := []struct {
		method      string
		routerPath  string
		testedPath  string
		httpHandler http.HandlerFunc
		expected    []byte
	}{
		{
			method:      "GET",
			routerPath:  "/status",
			testedPath:  "/status",
			httpHandler: provider.ApiGetStatus.Provider.HandlerFunc(),
			expected: func() []byte {
				status, err := provider.Comp.GetStatus("text", false)
				require.NoError(t, err)
				return status
			}(),
		},
		{
			method:      "GET",
			routerPath:  "/sections",
			testedPath:  "/sections",
			httpHandler: provider.ApiGetSectionList.Provider.HandlerFunc(),
			expected: func() []byte {
				res, err := json.Marshal(provider.Comp.GetSections())
				require.NoError(t, err)
				return res
			}(),
		},
		{
			method:      "GET",
			routerPath:  "/{component}/status",
			testedPath:  "/header/status",
			httpHandler: provider.APiGetSection.Provider.HandlerFunc(),
			expected: func() []byte {
				status, err := provider.Comp.GetStatusBySections([]string{"header"}, "text", false)
				require.NoError(t, err)
				return status
			}(),
		},
	}

	for _, test := range tests {
		t.Run(test.routerPath, func(t *testing.T) {
			r := mux.NewRouter()
			r.HandleFunc(test.routerPath, test.httpHandler)

			// Create a new HTTP request
			req, err := http.NewRequest(test.method, test.testedPath, nil)
			require.NoError(t, err)

			// Create a new HTTP response recorder
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			// Check the response status code
			require.Equal(t, rr.Code, http.StatusOK)

			// Check the response content type
			require.Equal(t, test.expected, rr.Body.Bytes())
		})
	}

}
