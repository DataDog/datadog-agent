// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomQueryCollectionIntervalConfiguration(t *testing.T) {
	tests := []struct {
		name      string
		instance  string
		init      string
		wantError string
	}{
		{
			name: "omitted",
			instance: `username: datadog
custom_queries:
  - query: SELECT 1 FROM dual
`,
		},
		{
			name: "positive",
			instance: `username: datadog
custom_queries:
  - query: SELECT 1 FROM dual
    collection_interval: 30
`,
		},
		{
			name: "zero",
			instance: `username: datadog
custom_queries:
  - query: SELECT 1 FROM dual
    collection_interval: 0
`,
			wantError: "custom_queries[0].collection_interval must be greater than zero",
		},
		{
			name: "negative global interval",
			instance: `username: datadog
`,
			init: `global_custom_queries:
  - query: SELECT 1 FROM dual
    collection_interval: -1
`,
			wantError: "global_custom_queries[0].collection_interval must be greater than zero",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := NewCheckConfig([]byte(tt.instance), []byte(tt.init))
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
				return
			}

			require.NoError(t, err)
			if tt.name == "omitted" {
				assert.Nil(t, cfg.InstanceConfig.CustomQueries[0].CollectionInterval)
			}
			if tt.name == "positive" {
				require.NotNil(t, cfg.InstanceConfig.CustomQueries[0].CollectionInterval)
				assert.EqualValues(t, 30, *cfg.InstanceConfig.CustomQueries[0].CollectionInterval)
			}
		})
	}
}

func TestServerEmbedsPortIsDerived(t *testing.T) {
	for name, tc := range map[string]struct {
		instance string
		embeds   bool
		server   string
		port     int
	}{
		"separate port":    {"username: u\nserver: db.example.com\nport: 1522\nservice_name: S\n", false, "db.example.com", 1522},
		"default port":     {"username: u\nserver: db.example.com\nservice_name: S\n", false, "db.example.com", 1521},
		"embedded port":    {"username: u\nserver: db.example.com:1522\nservice_name: S\n", true, "db.example.com", 1522},
		"both port forms":  {"username: u\nserver: db.example.com:1522\nport: 1523\nservice_name: S\n", true, "db.example.com", 1523},
		"yaml key ignored": {"username: u\nserver: db.example.com\nserver_embeds_port: true\nservice_name: S\n", false, "db.example.com", 1521},
	} {
		cfg, err := NewCheckConfig([]byte(tc.instance), []byte(""))
		require.NoError(t, err, name)
		assert.Equal(t, tc.embeds, cfg.ServerEmbedsPort, name)
		assert.Equal(t, tc.server, cfg.Server, name)
		assert.Equal(t, tc.port, cfg.Port, name)
	}
}
