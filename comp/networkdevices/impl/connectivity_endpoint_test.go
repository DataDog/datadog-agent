// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package networkdevicesimpl

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
)

func TestConnectivityCheckEndpointHandler(t *testing.T) {
	cases := []struct {
		name           string
		body           string
		expectedStatus int
	}{
		{
			name:           "rejects a malformed body",
			body:           "not-json",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "rejects an unsupported check",
			body:           `{"targets":["10.0.0.1"],"checks":["telnet"]}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "rejects an out-of-range snmp port",
			body:           `{"targets":["10.0.0.1"],"checks":["snmp"],"snmpOptions":{"port":65536}}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "accepts a request with no checks",
			body:           `{"targets":["10.0.0.1"],"checks":[]}`,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &networkDevicesImpl{logger: logmock.New(t)}

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/agent/networkdevices/connectivity-check", strings.NewReader(tc.body))
			c.ConnectivityCheckEndpointHandler()(w, r)

			require.Equal(t, tc.expectedStatus, w.Code)
		})
	}
}
