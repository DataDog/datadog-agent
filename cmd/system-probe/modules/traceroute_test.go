// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package modules

import (
	tracerouteutil "github.com/DataDog/datadog-agent/pkg/networkpath/traceroute"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestParseParams(t *testing.T) {
	tests := []struct {
		name           string
		vars           map[string]string
		expectedConfig tracerouteutil.Config
		expectedError  string
	}{
		{
			name: "only host",
			vars: map[string]string{
				"host": "1.2.3.4",
			},
			expectedConfig: tracerouteutil.Config{
				DestHostname: "1.2.3.4",
			},
		},
		{
			name: "all config",
			vars: map[string]string{
				"host":    "1.2.3.4",
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
			config, err := parseParams(tt.vars)
			assert.Equal(t, tt.expectedConfig, config)
			if tt.expectedError != "" {
				assert.EqualError(t, err, tt.expectedError)
			}
		})
	}
}
