// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package common

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var serialParams = TracerouteSerialParams{
	TracerouteParams{
		MinTTL:            1,
		MaxTTL:            10,
		TracerouteTimeout: 100 * time.Millisecond,
		PollFrequency:     1 * time.Millisecond,
		SendDelay:         1 * time.Millisecond,
	},
}
var serialInfo = TracerouteDriverInfo{
	SupportsParallel: false,
}

func TestTracerouteSerial(t *testing.T) {
	m := initMockDriver(t, serialParams.TracerouteParams, serialInfo)
	t.Parallel()

	var expectedResults []*ProbeResponse
	receiveProbes := make(chan *ProbeResponse, 1)

	sendTTL := uint8(0)
	m.sendHandler = func(ttl uint8) error {
		sendTTL++
		require.Equal(t, sendTTL, ttl)

		result := mockResult(sendTTL)
		if result != nil {
			expectedResults = append(expectedResults, result)
			select {
			case receiveProbes <- result:
			default:
				// we expect this channel to never fill
				t.Fatalf("TTL %d never got read by receiveHandler", sendTTL-1)
			}
		}

		return nil
	}
	m.receiveHandler = func() (*ProbeResponse, error) {
		var probe *ProbeResponse
		select {
		case probe = <-receiveProbes:
		default:
		}
		if probe == nil {
			return noData(serialParams.PollFrequency)
		}
		return probe, nil
	}
	results, err := TracerouteSerial(context.Background(), m, serialParams)
	require.NoError(t, err)
	require.Equal(t, expectedResults, results)
	require.Len(t, results, mockDestTTL)
}

func TestTracerouteSerialMissingHop(t *testing.T) {
	m := initMockDriver(t, serialParams.TracerouteParams, serialInfo)
	t.Parallel()

	var expectedResults []*ProbeResponse
	receiveProbes := make(chan *ProbeResponse, 1)

	sendTTL := uint8(0)
	m.sendHandler = func(ttl uint8) error {
		sendTTL++
		require.Equal(t, sendTTL, ttl)

		result := mockResult(sendTTL)
		if result != nil {
			// fake a missing hop
			if ttl == 2 {
				result = nil
			}
			expectedResults = append(expectedResults, result)
			select {
			case receiveProbes <- result:
			default:
				// we expect this channel to never fill
				t.Fatalf("TTL %d never got read by receiveHandler", sendTTL-1)
			}
		}

		return nil
	}
	m.receiveHandler = func() (*ProbeResponse, error) {
		var probe *ProbeResponse
		select {
		case probe = <-receiveProbes:
		default:
		}
		if probe == nil {
			return noData(serialParams.PollFrequency)
		}
		return probe, nil
	}
	results, err := TracerouteSerial(context.Background(), m, serialParams)
	require.NoError(t, err)
	require.Equal(t, expectedResults, results)
	require.Len(t, results, mockDestTTL)
}

func TestTracerouteSerialWrongHop(t *testing.T) {
	// this test checks that TracerouteSerial correctly handles a probe that returns the wrong hop
	m := initMockDriver(t, serialParams.TracerouteParams, serialInfo)
	t.Parallel()

	var expectedResults []*ProbeResponse
	receiveProbes := make(chan *ProbeResponse, 1)

	sendTTL := uint8(0)
	m.sendHandler = func(ttl uint8) error {
		sendTTL++
		require.Equal(t, sendTTL, ttl)

		result := mockResult(sendTTL)
		if result != nil {
			resultToRead := result
			// send the old hop twice
			if ttl == 2 {
				result = nil
				resultToRead = mockResult(sendTTL - 1)
			}
			expectedResults = append(expectedResults, result)
			select {
			case receiveProbes <- resultToRead:
			default:
				// we expect this channel to never fill
				t.Fatalf("TTL %d never got read by receiveHandler", sendTTL-1)
			}
		}

		return nil
	}
	m.receiveHandler = func() (*ProbeResponse, error) {
		var probe *ProbeResponse
		select {
		case probe = <-receiveProbes:
		default:
		}
		if probe == nil {
			return noData(serialParams.PollFrequency)
		}
		return probe, nil
	}
	results, err := TracerouteSerial(context.Background(), m, serialParams)
	require.NoError(t, err)
	require.Equal(t, expectedResults, results)
	require.Len(t, results, mockDestTTL)
}

func TestTracerouteSerialMissingDest(t *testing.T) {
	m := initMockDriver(t, serialParams.TracerouteParams, serialInfo)
	t.Parallel()

	var expectedResults []*ProbeResponse
	receiveProbes := make(chan *ProbeResponse, 1)

	sendTTL := uint8(0)
	m.sendHandler = func(ttl uint8) error {
		sendTTL++
		require.Equal(t, sendTTL, ttl)

		result := mockResult(sendTTL)
		if result != nil {
			// fake a missing destination
			if result.IsDest {
				result = nil
			}
			expectedResults = append(expectedResults, result)
			select {
			case receiveProbes <- result:
			default:
				// we expect this channel to never fill
				t.Fatalf("TTL %d never got read by receiveHandler", sendTTL-1)
			}
		}

		return nil
	}
	m.receiveHandler = func() (*ProbeResponse, error) {
		var probe *ProbeResponse
		select {
		case probe = <-receiveProbes:
		default:
		}
		if probe == nil {
			return noData(serialParams.PollFrequency)
		}
		return probe, nil
	}
	results, err := TracerouteSerial(context.Background(), m, serialParams)
	require.NoError(t, err)

	require.Len(t, results, int(parallelParams.MaxTTL))
	for i, r := range results {
		// up to but excluding the destination TTL, we should have results
		if i < len(expectedResults) {
			require.Equal(t, expectedResults[i], r, "mismatch at index %d", i)
		} else {
			// after that, it should all be zero up to MaxTTL
			require.Zero(t, r, "expected zero at index %d", i)
		}
	}
}

func TestTracerouteSerialSendErr(t *testing.T) {
	// this test checks that TracerouteSerial returns an error if SendProbe() fails
	m := initMockDriver(t, serialParams.TracerouteParams, serialInfo)
	t.Parallel()

	hasCalled := false
	m.sendHandler = func(_ uint8) error {
		require.False(t, hasCalled, "SendProbe() called more than once")
		hasCalled = true

		return errMock
	}

	results, err := TracerouteSerial(context.Background(), m, serialParams)
	require.Nil(t, results)
	require.ErrorIs(t, err, errMock)
}

func TestTracerouteSerialReceiveErr(t *testing.T) {
	// this test checks that TracerouteSerial returns an error if ReceiveProbe() fails
	m := initMockDriver(t, serialParams.TracerouteParams, serialInfo)
	t.Parallel()

	hasCalled := false
	m.receiveHandler = func() (*ProbeResponse, error) {
		require.False(t, hasCalled, "ReceiveProbe() called more than once")
		hasCalled = true

		return nil, errMock
	}

	results, err := TracerouteSerial(context.Background(), m, serialParams)
	require.Nil(t, results)
	require.ErrorIs(t, err, errMock)
}
