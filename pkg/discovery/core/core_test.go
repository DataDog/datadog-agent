// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package core

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPidSet(t *testing.T) {
	set := make(PidSet)

	t.Run("empty set", func(t *testing.T) {
		assert.False(t, set.Has(123))
	})

	t.Run("add and remove", func(t *testing.T) {
		set.Add(123)
		assert.True(t, set.Has(123))

		set.Remove(123)
		assert.False(t, set.Has(123))
	})

	t.Run("multiple pids", func(t *testing.T) {
		set := make(PidSet)
		set.Add(100)
		set.Add(200)
		set.Add(300)

		assert.True(t, set.Has(100))
		assert.True(t, set.Has(200))
		assert.True(t, set.Has(300))
		assert.False(t, set.Has(400))
	})
}

func TestDiscovery_Close(t *testing.T) {
	// Test that Close can be called without panic
	d := &Discovery{
		Config: &DiscoveryConfig{},
	}
	d.Close() // should not panic
}

func TestParams_ToJSON(t *testing.T) {
	tests := []struct {
		name     string
		params   Params
		expected string
	}{
		{
			name:     "empty params",
			params:   Params{},
			expected: `{}`,
		},
		{
			name: "with new pids only",
			params: Params{
				NewPids: []int32{1, 2, 3},
			},
			expected: `{"new_pids":[1,2,3]}`,
		},
		{
			name: "with heartbeat pids only",
			params: Params{
				HeartbeatPids: []int32{100, 200},
			},
			expected: `{"heartbeat_pids":[100,200]}`,
		},
		{
			name: "with both new and heartbeat pids",
			params: Params{
				NewPids:       []int32{1, 2},
				HeartbeatPids: []int32{100, 200},
			},
			expected: `{"new_pids":[1,2],"heartbeat_pids":[100,200]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.params.ToJSON()
			require.NoError(t, err)
			assert.JSONEq(t, tt.expected, string(data))
		})
	}
}

func TestFromJSON(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    Params
		expectError bool
	}{
		{
			name:     "empty object",
			input:    `{}`,
			expected: Params{},
		},
		{
			name:  "with new pids",
			input: `{"new_pids":[1,2,3]}`,
			expected: Params{
				NewPids: []int32{1, 2, 3},
			},
		},
		{
			name:  "with heartbeat pids",
			input: `{"heartbeat_pids":[100,200]}`,
			expected: Params{
				HeartbeatPids: []int32{100, 200},
			},
		},
		{
			name:  "with both types of pids",
			input: `{"new_pids":[1,2],"heartbeat_pids":[100,200]}`,
			expected: Params{
				NewPids:       []int32{1, 2},
				HeartbeatPids: []int32{100, 200},
			},
		},
		{
			name:        "invalid json",
			input:       `{invalid`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, err := FromJSON([]byte(tt.input))
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, params)
			}
		})
	}
}

func TestParseParamsFromRequest(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		expected    Params
		expectError bool
	}{
		{
			name:     "nil body",
			body:     "",
			expected: Params{},
		},
		{
			name: "valid json body",
			body: `{"new_pids":[1,2,3]}`,
			expected: Params{
				NewPids: []int32{1, 2, 3},
			},
		},
		{
			name:        "invalid json body",
			body:        `{invalid`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body io.Reader
			if tt.body != "" {
				body = bytes.NewBufferString(tt.body)
			}
			req, err := http.NewRequest(http.MethodPost, "/test", body)
			require.NoError(t, err)

			params, err := ParseParamsFromRequest(req)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, params)
			}
		})
	}
}

func TestLoadIgnoredComms(t *testing.T) {
	tests := []struct {
		name     string
		comms    []string
		expected map[string]struct{}
	}{
		{
			name:     "empty list",
			comms:    []string{},
			expected: nil, // loadIgnoredComms returns early when empty
		},
		{
			name:  "single command",
			comms: []string{"bash"},
			expected: map[string]struct{}{
				"bash": {},
			},
		},
		{
			name:  "multiple commands",
			comms: []string{"bash", "sh", "zsh"},
			expected: map[string]struct{}{
				"bash": {},
				"sh":   {},
				"zsh":  {},
			},
		},
		{
			name:  "command exceeding MaxCommLen gets truncated",
			comms: []string{"verylongcommandname"}, // > 15 chars
			expected: map[string]struct{}{
				"verylongcommand": {}, // truncated to 15 chars
			},
		},
		{
			name:  "command exactly at MaxCommLen",
			comms: []string{"fifteencharact"}, // exactly 14 chars (< 15)
			expected: map[string]struct{}{
				"fifteencharact": {},
			},
		},
		{
			name:  "empty string in list is ignored",
			comms: []string{"bash", "", "sh"},
			expected: map[string]struct{}{
				"bash": {},
				"sh":   {},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &DiscoveryConfig{}
			config.loadIgnoredComms(tt.comms)
			assert.Equal(t, tt.expected, config.IgnoreComms)
		})
	}
}

func TestJoin(t *testing.T) {
	tests := []struct {
		name     string
		pieces   []string
		expected string
	}{
		{
			name:     "empty pieces",
			pieces:   []string{},
			expected: "",
		},
		{
			name:     "single piece",
			pieces:   []string{"discovery"},
			expected: "discovery",
		},
		{
			name:     "multiple pieces",
			pieces:   []string{"discovery", "ignored_command_names"},
			expected: "discovery.ignored_command_names",
		},
		{
			name:     "three pieces",
			pieces:   []string{"a", "b", "c"},
			expected: "a.b.c",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := join(tt.pieces...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMaxCommLen(t *testing.T) {
	// Verify MaxCommLen constant is 15 as documented
	assert.Equal(t, 15, MaxCommLen)
}

func TestHeartbeatTime(t *testing.T) {
	// Verify HeartbeatTime is 15 minutes
	assert.Equal(t, int64(15*60*1e9), HeartbeatTime.Nanoseconds())
}
