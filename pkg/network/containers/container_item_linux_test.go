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

	"github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/require"
	"go4.org/intern"

	"github.com/DataDog/datadog-agent/pkg/network/events"
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

// mockResolvConfReader is a mock implementation of resolvConfReader for testing
type mockResolvConfReader struct {
	result string
	err    error
}

func (m *mockResolvConfReader) readResolvConf(_ *events.Process) (string, error) {
	return m.result, m.err
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
			name:     "ErrNotExist",
			err:      os.ErrNotExist,
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := errIsProcessNotRunning(tt.err)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestReadContainerItemProcessRunningVsNotRunning(t *testing.T) {
	tests := []struct {
		name                     string
		isProcessRunning         bool
		isProcessStillRunningErr error
		readResolvConfResult     string
		readResolvConfErr        error
		expectedNoDataReason     string
		expectedError            bool
		expectedResolvConf       string
	}{
		{
			name:                     "process not running - should return noDataReason",
			isProcessRunning:         false,
			isProcessStillRunningErr: nil,
			readResolvConfResult:     "nameserver 8.8.8.8",
			readResolvConfErr:        nil,
			expectedNoDataReason:     "process not running",
			expectedError:            false,
			expectedResolvConf:       "",
		},
		{
			name:                     "process running - should return resolv.conf data",
			isProcessRunning:         true,
			isProcessStillRunningErr: nil,
			readResolvConfResult:     "nameserver 8.8.8.8",
			readResolvConfErr:        nil,
			expectedNoDataReason:     "",
			expectedError:            false,
			expectedResolvConf:       "nameserver 8.8.8.8",
		},
		{
			name:                     "process running but resolv.conf read failed",
			isProcessRunning:         true,
			isProcessStillRunningErr: nil,
			readResolvConfResult:     "",
			readResolvConfErr:        errors.New("permission denied"),
			expectedNoDataReason:     "",
			expectedError:            true,
			expectedResolvConf:       "",
		},
		{
			name:                     "isProcessStillRunning returns error",
			isProcessRunning:         false,
			isProcessStillRunningErr: errors.New("process check failed"),
			readResolvConfResult:     "nameserver 8.8.8.8",
			readResolvConfErr:        nil,
			expectedNoDataReason:     "",
			expectedError:            true,
			expectedResolvConf:       "",
		},
		{
			name:                     "process running with empty resolv.conf",
			isProcessRunning:         true,
			isProcessStillRunningErr: nil,
			readResolvConfResult:     "",
			readResolvConfErr:        nil,
			expectedNoDataReason:     "",
			expectedError:            false,
			expectedResolvConf:       "",
		},
		{
			name:                     "readResolvConf ErrorProcessNotRunning + process not running",
			isProcessRunning:         false,
			isProcessStillRunningErr: nil,
			readResolvConfResult:     "",
			readResolvConfErr:        process.ErrorProcessNotRunning,
			expectedNoDataReason:     "process not running",
			expectedError:            false,
			expectedResolvConf:       "",
		},
		{
			name:                     "readResolvConf ErrProcessDone + process not running",
			isProcessRunning:         false,
			isProcessStillRunningErr: nil,
			readResolvConfResult:     "",
			readResolvConfErr:        os.ErrProcessDone,
			expectedNoDataReason:     "process not running",
			expectedError:            false,
			expectedResolvConf:       "",
		},
		{
			name:                     "readResolvConf wrapped ErrorProcessNotRunning + process not running",
			isProcessRunning:         false,
			isProcessStillRunningErr: nil,
			readResolvConfResult:     "",
			readResolvConfErr:        errors.Join(errors.New("context"), process.ErrorProcessNotRunning),
			expectedNoDataReason:     "process not running",
			expectedError:            false,
			expectedResolvConf:       "",
		},
		{
			name:                     "readResolvConf wrapped ErrProcessDone + process not running",
			isProcessRunning:         false,
			isProcessStillRunningErr: nil,
			readResolvConfResult:     "",
			readResolvConfErr:        errors.Join(errors.New("wrapped"), os.ErrProcessDone),
			expectedNoDataReason:     "process not running",
			expectedError:            false,
			expectedResolvConf:       "",
		},
		{
			name:                     "readResolvConf ErrNotExist + process running should propagate",
			isProcessRunning:         true,
			isProcessStillRunningErr: nil,
			readResolvConfResult:     "",
			readResolvConfErr:        os.ErrNotExist,
			expectedNoDataReason:     "",
			expectedError:            true,
			expectedResolvConf:       "",
		},
		{
			name:                     "readResolvConf ErrNotExist + process not running should not propagate",
			isProcessRunning:         false,
			isProcessStillRunningErr: nil,
			readResolvConfResult:     "",
			readResolvConfErr:        os.ErrNotExist,
			expectedNoDataReason:     "process not running",
			expectedError:            false,
			expectedResolvConf:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr := newContainerReader(
				&mockResolvConfReader{
					result: tt.readResolvConfResult,
					err:    tt.readResolvConfErr,
				},
			)
			// Override isProcessStillRunning for mocking
			cr.isProcessStillRunning = func(_ context.Context, _ *events.Process) (bool, error) {
				return tt.isProcessRunning, tt.isProcessStillRunningErr
			}

			processEvent := &events.Process{
				Pid:         12345,
				ContainerID: intern.GetByString("test-container"),
			}

			result, err := cr.readContainerItem(context.Background(), processEvent)

			if tt.expectedError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedNoDataReason, result.noDataReason)
				if tt.expectedResolvConf != "" {
					require.Equal(t, tt.expectedResolvConf, result.item.resolvConf.Get())
				} else {
					require.Empty(t, result.item.resolvConf)
				}
			}
		})
	}
}
