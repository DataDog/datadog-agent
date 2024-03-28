// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package guiimpl

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func Test_makeFlare(t *testing.T) {
	tests := []struct {
		name         string
		payload      string
		expectedBody string
	}{
		{
			name:         "Bad payload",
			payload:      "a\n",
			expectedBody: "invalid character 'a' looking for beginning of value",
		},
		{
			name:         "Missing email",
			payload:      "{\"caseID\": \"102123123\"}",
			expectedBody: "Error creating flare: missing information",
		},
		{
			name:         "Missing caseID",
			payload:      "{\"email\": \"test@example.com\"}",
			expectedBody: "Error creating flare: missing information",
		},
		{
			name:         "Invalid caseID",
			payload:      "{\"email\": \"test@example.com\", \"caseID\": \"102123123a\"}",
			expectedBody: "Invalid CaseID (must be a number)",
		},
	}

	fakeGuiStartTimestamp := time.Now().Unix()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("POST", "/flare", strings.NewReader(tt.payload))
			require.NoError(t, err)

			rr := httptest.NewRecorder()

			router := mux.NewRouter()
			agentHandler(router, nil, nil, nil, fakeGuiStartTimestamp)
			router.ServeHTTP(rr, req)

			resp := rr.Result()
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			assert.Equal(t, string(body), tt.expectedBody)
		})
	}
}

func Test_getConfigSetting(t *testing.T) {
	tests := []struct {
		name          string
		configSetting string
		configValue   string
		expectedBody  string
	}{
		{
			name:          "Allowed setting",
			configSetting: "apm_config.receiver_port",
			configValue:   "8126",
			expectedBody:  "{\"apm_config.receiver_port\":8126}\n",
		},
		{
			name:          "Not allowed setting",
			configSetting: "api_key",
			configValue:   "",
			expectedBody:  "\"error\": \"requested setting is not whitelisted\"",
		},
	}

	fakeGuiStartTimestamp := time.Now().Unix()

	c := fxutil.Test[config.Component](t,
		config.MockModule(),
	)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.configValue != "" {
				c.SetWithoutSource(tt.configSetting, tt.configValue)
			}

			path := fmt.Sprintf("/getConfig/%s", tt.configSetting)
			req, err := http.NewRequest("GET", path, nil)
			require.NoError(t, err)

			rr := httptest.NewRecorder()

			router := mux.NewRouter()
			agentHandler(router, nil, nil, c, fakeGuiStartTimestamp)
			router.ServeHTTP(rr, req)

			resp := rr.Result()
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			assert.Equal(t, string(body), tt.expectedBody)
		})
	}
}
