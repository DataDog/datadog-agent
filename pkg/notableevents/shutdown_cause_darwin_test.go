// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package notableevents

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePMUBootFaultPayload(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		expect  []string
	}{
		{
			name:    "single token",
			payload: "target_off_restart",
			expect:  []string{"target_off_restart"},
		},
		{
			name:    "several tokens",
			payload: "rst\x1fbtn_rst,btn_seq_reset\x1ftarget_off_restart\x1fwdog,reset_in_1",
			expect: []string{
				"rst", "btn_rst,btn_seq_reset", "target_off_restart", "wdog,reset_in_1",
			},
		},
		{
			name:    "empty tokens are dropped",
			payload: "\x1frst\x1f\x1fwdog,reset_in_1\x1f",
			expect:  []string{"rst", "wdog,reset_in_1"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expect, parsePMUBootFaultPayload(test.payload).Tokens)
		})
	}
}

// TestReadPMUBootFaultInfoDegradesGracefully performs the real, read-only
// IORegistry read. The property is absent on virtualized macOS, so the only
// portable assertion is that an absent or empty payload is not an error.
func TestReadPMUBootFaultInfoDegradesGracefully(t *testing.T) {
	info, err := readPMUBootFaultInfo()
	if runtime.GOARCH != "arm64" {
		require.ErrorIs(t, err, errShutdownCauseUnsupported)
		return
	}
	require.NoError(t, err)

	for _, token := range info.Tokens {
		assert.NotEmpty(t, token)
		assert.NotContains(t, token, pmuTokenSeparator)
	}
}

func TestReadBootSessionUUID(t *testing.T) {
	uuid, err := readBootSessionUUID()
	require.NoError(t, err)
	assert.Len(t, uuid, 36)
}

func TestReadBootTime(t *testing.T) {
	bootTime, err := readBootTime()
	require.NoError(t, err)
	assert.False(t, bootTime.IsZero())
	assert.Equal(t, time.UTC, bootTime.Location())
	assert.True(t, bootTime.Before(time.Now()), "boot time must be in the past")
}
