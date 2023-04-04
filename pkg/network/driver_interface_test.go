// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm
// +build windows,npm

package network

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

type TestDriverHandleInfiniteLoop struct {
	t *testing.T
	// state variables
	hasBeenCalled  bool
	lastBufferSize int
}

func (tdh *TestDriverHandleInfiniteLoop) ReadFile(p []byte, bytesRead *uint32, ol *windows.Overlapped) error {
	// check state in struct to see if we've been called before
	if tdh.hasBeenCalled {
		// TODO: verify <= is correct as opposed to ==
		if len(p) <= tdh.lastBufferSize {
			tdh.t.Fatal("Consecutive calls without a larger buffer")
		}
		return nil
	}
	tdh.hasBeenCalled = true
	*bytesRead = 0
	tdh.lastBufferSize = len(p)
	return windows.ERROR_MORE_DATA
}

func (tdh *TestDriverHandleInfiniteLoop) GetWindowsHandle() windows.Handle {
	return windows.Handle(0)
}

func (tdh *TestDriverHandleInfiniteLoop) DeviceIoControl(ioControlCode uint32, inBuffer *byte, inBufferSize uint32, outBuffer *byte, outBufferSize uint32, bytesReturned *uint32, overlapped *windows.Overlapped) (err error) {
	return nil
}

func (tdh *TestDriverHandleInfiniteLoop) CancelIoEx(ol *windows.Overlapped) error {
	return nil
}

func (tdh *TestDriverHandleInfiniteLoop) GetStatsForHandle() (map[string]map[string]int64, error) {
	return nil, nil
}

func (tdh *TestDriverHandleInfiniteLoop) Close() error {
	return nil
}

func TestConnectionStatsInfiniteLoop(t *testing.T) {

	startSize := 10
	minSize := 10

	activeBuf := NewConnectionBuffer(startSize, minSize)
	closedBuf := NewConnectionBuffer(startSize, minSize)

	di, err := NewDriverInterface(config.New(), func(flags uint32, handleType driver.HandleType) (driver.Handle, error) {
		return &TestDriverHandleInfiniteLoop{t: t}, nil
	})
	require.NoError(t, err, "Failed to create new driver interface")

	_, err = di.GetClosedConnectionStats(closedBuf, func(c *ConnectionStats) bool {
		return true
	})
	require.NoError(t, err, "Failed to get connection stats")
	_, err = di.GetOpenConnectionStats(activeBuf, func(c *ConnectionStats) bool {
		return true
	})
	require.NoError(t, err, "Failed to get connection stats")
}
