// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package network

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/driver"
)

type TestDriverHandleInfiniteLoop struct {
	t *testing.T
	// state variables
	hasBeenCalled  bool
	lastBufferSize int
}

func (tdh *TestDriverHandleInfiniteLoop) RefreshStats() {}

//nolint:revive // TODO(WKIT) Fix revive linter
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

//nolint:revive // TODO(WKIT) Fix revive linter
func (tdh *TestDriverHandleInfiniteLoop) SynchronousDeviceIoControl(ioControlCode uint32, inBuffer *byte, inBufferSize uint32, outBuffer *byte, outBufferSize uint32) (bytesReturned uint32, err error) {
	return 0, nil
}

// Deprecated: matches the deprecated shim on driver.Handle so this mock still
// satisfies the interface; remove together with the production shim.
//
//nolint:revive // TODO(WKIT) Fix revive linter
func (tdh *TestDriverHandleInfiniteLoop) DeviceIoControl(ioControlCode uint32, inBuffer *byte, inBufferSize uint32, outBuffer *byte, outBufferSize uint32, bytesReturned *uint32, overlapped *windows.Overlapped) error {
	return nil
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (tdh *TestDriverHandleInfiniteLoop) CancelIoEx(ol *windows.Overlapped) error {
	return nil
}

func (tdh *TestDriverHandleInfiniteLoop) Close() error {
	return nil
}

func TestConnectionStatsInfiniteLoop(t *testing.T) {

	startSize := 10
	minSize := 10

	activeBuf := NewConnectionBuffer(startSize, minSize)
	closedBuf := NewConnectionBuffer(startSize, minSize)

	di, err := NewDriverInterface(config.New(), func(_ uint32, _ driver.HandleType, _ telemetry.Component) (driver.Handle, error) {
		return &TestDriverHandleInfiniteLoop{t: t}, nil
	}, nil)
	require.NoError(t, err, "Failed to create new driver interface")

	_, err = di.GetClosedConnectionStats(closedBuf, func(_ *ConnectionStats) bool {
		return true
	})
	require.NoError(t, err, "Failed to get connection stats")
	_, err = di.GetOpenConnectionStats(activeBuf, func(_ *ConnectionStats) bool {
		return true
	})
	require.NoError(t, err, "Failed to get connection stats")
}

// TestClassifiedProtocolName covers the telemetry tag values derived from the
// driver classification, which drive the windows.classified_flows counter.
func TestClassifiedProtocolName(t *testing.T) {
	tests := []struct {
		name             string
		status           uint16
		classifyRequest  uint16
		classifyResponse uint16
		expected         string
	}{
		{"redis", driver.ClassificationClassified, driver.ClassificationRequestRedis, 0, "redis"},
		{"postgres", driver.ClassificationClassified, driver.ClassificationRequestPostgres, 0, "postgres"},
		{"mysql", driver.ClassificationClassified, driver.ClassificationRequestMySQL, 0, "mysql"},
		{"mongo", driver.ClassificationClassified, driver.ClassificationRequestMongo, 0, "mongo"},
		{"amqp", driver.ClassificationClassified, driver.ClassificationRequestAMQP, 0, "amqp"},
		{"http", driver.ClassificationClassified, driver.ClassificationRequestHTTPGet, 0, "http"},
		{"http delete", driver.ClassificationClassified, driver.ClassificationRequestHTTPDelete, 0, "http"},
		{"http2", driver.ClassificationClassified, driver.ClassificationRequestHTTP2, 0, "http2"},
		{"tls", driver.ClassificationClassified, driver.ClassificationRequestTLS, 0, "tls"},
		{
			"response only http",
			driver.ClassificationClassified,
			driver.ClassificationRequestUnclassified,
			driver.ClassificationResponseHTTP,
			"http",
		},
		// unclassified flows must not be counted, otherwise the survey is diluted
		{"unclassified", driver.ClassificationUnclassified, driver.ClassificationRequestRedis, 0, ""},
		{"unknown", driver.ClassificationUnknown, driver.ClassificationRequestUnclassified, 0, ""},
		{
			"insufficient data",
			driver.ClassificationUnableInsufficientData,
			driver.ClassificationRequestUnclassified,
			0,
			"",
		},
		{
			"classified but no value set",
			driver.ClassificationClassified,
			driver.ClassificationRequestUnclassified,
			driver.ClassificationResponseUnclassified,
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flow := &driver.PerFlowData{
				ClassificationStatus: tt.status,
				ClassifyRequest:      tt.classifyRequest,
				ClassifyResponse:     tt.classifyResponse,
			}

			assert.Equal(t, tt.expected, classifiedProtocolName(flow))
		})
	}
}
