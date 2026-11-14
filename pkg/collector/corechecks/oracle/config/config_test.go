// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package config

import (
	"strconv"
	"testing"
	"time"

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

func TestQueryTimeoutBounds(t *testing.T) {
	for _, configured := range []int{0, -1, -int(^uint(0)>>1) - 1, 1, 20, 60, int(^uint(0) >> 1)} {
		t.Run(strconv.Itoa(configured), func(t *testing.T) {
			c := InstanceConfig{ConnectionConfig: ConnectionConfig{QueryTimeout: configured}}
			q := c.QueryTimeoutDuration()
			require.Positive(t, q)
			require.Positive(t, 2*q)
			seconds, err := strconv.ParseInt(c.QueryTimeoutString(), 10, 64)
			require.NoError(t, err)
			socket, err := strconv.ParseInt(c.SocketTimeoutString(), 10, 64)
			require.NoError(t, err)
			require.Equal(t, q, time.Duration(seconds)*time.Second)
			require.Equal(t, 2*q, time.Duration(socket)*time.Second)
			if configured <= 0 {
				require.Equal(t, 20*time.Second, q)
			}
			if configured > 0 && configured <= 60 {
				require.Equal(t, time.Duration(configured)*time.Second, q)
			}
		})
	}
	c, err := NewCheckConfig([]byte("username: datadog"), []byte("{}"))
	require.NoError(t, err)
	require.Equal(t, 20, c.QueryTimeout)
	require.Equal(t, 20*time.Second, c.QueryTimeoutDuration())
}

func TestNativeQueryTimeoutDoesNotOverflowOCI(t *testing.T) {
	for _, connection := range []ConnectionConfig{
		{OracleClient: true, QueryTimeout: int(^uint(0) >> 1)},
		{TnsAlias: "test", QueryTimeout: int(^uint(0) >> 1)},
	} {
		c := InstanceConfig{ConnectionConfig: connection}
		q := c.QueryTimeoutDuration()
		require.Positive(t, q)
		require.LessOrEqual(t, int64(q/time.Millisecond), int64(1<<32-1))
		require.Equal(t, "4294967", c.QueryTimeoutString())
		require.Equal(t, "8589934", c.SocketTimeoutString())
	}
}
