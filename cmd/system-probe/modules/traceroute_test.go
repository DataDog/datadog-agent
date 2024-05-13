// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package modules

import (
	"context"
	"net/http"
	"testing"

	tracerouteutil "github.com/DataDog/datadog-agent/pkg/networkpath/traceroute"
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
				"port":    "42",
				"max_ttl": "35",
				"timeout": "1000",
			},
			expectedConfig: tracerouteutil.Config{
				DestHostname: "1.2.3.4",
				DestPort:     42,
				MaxTTL:       35,
				TimeoutMs:    1000,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t1 *testing.T) {
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
