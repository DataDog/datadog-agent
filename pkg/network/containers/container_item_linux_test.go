// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package containers

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/require"
	"go4.org/intern"

	"github.com/DataDog/datadog-agent/pkg/network/events"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestStripResolvConf(t *testing.T) {
	resolvConf := `
; comment goes here
# other comment goes here
nameserver 8.8.8.8
	# indented comment with spaces
	nameserver 8.8.4.4  
`
	reader := strings.NewReader(resolvConf)

	rs := makeResolvStripper(resolvConfInputMaxSizeBytes)
	stripped, err := rs.stripResolvConf(len(resolvConf), reader)
	require.NoError(t, err)

	require.Equal(t, "nameserver 8.8.8.8\nnameserver 8.8.4.4", stripped)
}

// errorReader is a fake reader that returns a specific error
type errorReader struct {
	err error
}

func (er *errorReader) Read(_ []byte) (n int, err error) {
	return 0, er.err
}
func TestStripResolvConfReaderError(t *testing.T) {
	customErr := errors.New("custom read error")
	reader := &errorReader{err: customErr}

	rs := makeResolvStripper(resolvConfInputMaxSizeBytes)
	_, err := rs.stripResolvConf(100, reader)
	require.Error(t, err)
	require.ErrorIs(t, err, customErr)
}

func TestStripResolvConfTooBigInput(t *testing.T) {
	resolvConf := strings.Repeat("a", 5000)
	reader := strings.NewReader(resolvConf)

	rs := makeResolvStripper(resolvConfInputMaxSizeBytes)
	stripped, err := rs.stripResolvConf(len(resolvConf), reader)
	require.NoError(t, err)

	require.Equal(t, "<too big: kind=input size=5000>", stripped)
}
func TestStripResolvConfTooBigOutput(t *testing.T) {
	resolvConf := strings.Repeat("a", 2000)
	reader := strings.NewReader(resolvConf)

	rs := makeResolvStripper(resolvConfInputMaxSizeBytes)
	stripped, err := rs.stripResolvConf(len(resolvConf), reader)
	require.NoError(t, err)

	require.Equal(t, "<too big: kind=output size=2000>", stripped)
}

func TestCheckProcessNotRunning(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "ErrorProcessNotRunning",
			err:      process.ErrorProcessNotRunning,
			expected: true,
		},
		{
			name:     "ErrProcessDone",
			err:      os.ErrProcessDone,
			expected: true,
		},
		{
			name:     "wrapped ErrorProcessNotRunning",
			err:      errors.Join(errors.New("context"), process.ErrorProcessNotRunning),
			expected: true,
		},
		{
			name:     "wrapped ErrProcessDone",
			err:      errors.Join(errors.New("context"), os.ErrProcessDone),
			expected: true,
		},
		{
			name:     "other error",
			err:      errors.New("some other error"),
			expected: false,
		},
		{
			name:     "ErrNotExist should not match",
			err:      os.ErrNotExist,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := errIsProcessNotRunning(tt.err)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestReadContainerItemProcessNotRunningFromReadResolvConf(t *testing.T) {
	tests := []struct {
		name              string
		readResolvConfErr error
		expectedReason    string
	}{
		{
			name:              "ErrorProcessNotRunning",
			readResolvConfErr: process.ErrorProcessNotRunning,
			expectedReason:    "process not running (when reading resolv.conf)",
		},
		{
			name:              "ErrProcessDone",
			readResolvConfErr: os.ErrProcessDone,
			expectedReason:    "process not running (when reading resolv.conf)",
		},
		{
			name:              "wrapped ErrorProcessNotRunning",
			readResolvConfErr: errors.Join(errors.New("context"), process.ErrorProcessNotRunning),
			expectedReason:    "process not running (when reading resolv.conf)",
		},
		{
			name:              "wrapped ErrProcessDone",
			readResolvConfErr: errors.Join(errors.New("wrapped"), os.ErrProcessDone),
			expectedReason:    "process not running (when reading resolv.conf)",
		},
		{
			name:              "other error should propagate",
			readResolvConfErr: errors.New("permission denied"),
			expectedReason:    "",
		},
		{
			name:              "ErrNotExist should propagate",
			readResolvConfErr: os.ErrNotExist,
			expectedReason:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr := containerReader{
				resolvStripper: makeResolvStripper(resolvConfInputMaxSizeBytes),
				readResolvConf: func(_ *events.Process) (string, error) {
					return "", tt.readResolvConfErr
				},
				debugLimit: log.NewLogLimit(999, time.Second),
			}

			processEvent := &events.Process{
				Pid:         12345,
				ContainerID: intern.GetByString("test-container"),
			}

			result, err := cr.readContainerItem(context.Background(), processEvent)

			if tt.expectedReason != "" {
				// Should return no error, but have a noDataReason
				require.NoError(t, err)
				require.Equal(t, tt.expectedReason, result.noDataReason)
				require.Empty(t, result.item.resolvConf)
			} else {
				// Should propagate the error
				require.Error(t, err)
				require.ErrorIs(t, err, tt.readResolvConfErr)
				require.Empty(t, result.noDataReason)
			}
		})
	}
}
