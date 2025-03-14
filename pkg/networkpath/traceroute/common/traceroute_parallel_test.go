// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package common

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type MockDriver struct {
	t      *testing.T
	params TracerouteParallelParams

	sentTTLs map[uint8]struct{}

	sendHandler    func(ttl uint8) error
	receiveHandler func() (*ProbeResponse, error)
}

func initMockDriver(t *testing.T, params TracerouteParallelParams) *MockDriver {
	return &MockDriver{
		t:              t,
		params:         params,
		sentTTLs:       make(map[uint8]struct{}),
		sendHandler:    nil,
		receiveHandler: nil,
	}
}

func (m *MockDriver) SendProbe(ttl uint8) error {
	require.NotContains(m.t, m.sentTTLs, ttl, "same TTL sent twice")
	m.sentTTLs[ttl] = struct{}{}

	m.t.Logf("wrote %d\n", ttl)
	if m.sendHandler == nil {
		return nil
	}
	return m.sendHandler(ttl)
}

func (m *MockDriver) ReceiveProbe(timeout time.Duration) (*ProbeResponse, error) {
	require.Equal(m.t, m.params.PollFrequency, timeout)

	if m.receiveHandler == nil {
		return nil, ErrReceiveProbeNoPkt
	}
	res, err := m.receiveHandler()
	if !errors.Is(err, ErrReceiveProbeNoPkt) {
		m.t.Logf("read %+v, %v\n", res, err)
	}
	return res, err
}

func noData(pollFrequency time.Duration) (*ProbeResponse, error) {
	time.Sleep(pollFrequency)
	return nil, ErrReceiveProbeNoPkt
}

var testParams = TracerouteParallelParams{
	MinTTL:            1,
	MaxTTL:            10,
	TracerouteTimeout: 50 * time.Millisecond,
	PollFrequency:     1 * time.Millisecond,
	SendDelay:         1 * time.Millisecond,
}

const mockDestTTL = 5

func mockResult(ttl uint8) *ProbeResponse {
	if ttl > mockDestTTL {
		return nil
	}
	return &ProbeResponse{
		TTL: ttl,
		IP:  netip.AddrFrom4([4]byte{10, 0, 0, ttl}),
		// mock RTT as the TTL in milliseconds
		RTT:    time.Duration(ttl) * time.Millisecond,
		IsDest: ttl == mockDestTTL,
	}
}

func TestParallelTraceroute(t *testing.T) {
	// basic test that checks if the traceroute runs correctly
	m := initMockDriver(t, testParams)

	var expectedResults []*ProbeResponse
	receiveProbes := make(chan *ProbeResponse, testParams.MaxTTL)

	expectedTTL := uint8(1)
	m.sendHandler = func(ttl uint8) error {
		require.Equal(t, expectedTTL, ttl)
		expectedTTL++

		result := mockResult(ttl)

		if result != nil {
			expectedResults = append(expectedResults, result)
			receiveProbes <- result
			if result.IsDest {
				close(receiveProbes)
			}
		}

		return nil
	}
	m.receiveHandler = func() (*ProbeResponse, error) {
		probe, ok := <-receiveProbes
		if !ok {
			return noData(testParams.PollFrequency)
		}
		return probe, nil
	}

	results, err := TracerouteParallel(context.Background(), m, testParams)
	require.NoError(t, err)
	require.Equal(t, expectedResults, results)
	require.Len(t, results, mockDestTTL)
}

func testParallelTracerouteShuffled(t *testing.T, seed int64) {
	// similar to TestParallelTraceroute, except it shuffles the received probes
	// and expects them to come back in the correct order
	r := rand.New(rand.NewSource(seed))

	m := initMockDriver(t, testParams)

	var expectedResults []*ProbeResponse
	receiveProbes := make(chan *ProbeResponse, testParams.MaxTTL)

	m.sendHandler = func(ttl uint8) error {
		result := mockResult(ttl)
		if result != nil {
			expectedResults = append(expectedResults, result)

			if result.IsDest {
				for _, p := range r.Perm(len(expectedResults)) {
					receiveProbes <- expectedResults[p]
				}
				close(receiveProbes)
			}
		}

		return nil
	}
	m.receiveHandler = func() (*ProbeResponse, error) {
		probe, ok := <-receiveProbes
		if !ok {
			return noData(testParams.PollFrequency)
		}
		return probe, nil
	}

	results, err := TracerouteParallel(context.Background(), m, testParams)
	require.NoError(t, err)
	require.Equal(t, expectedResults, results)
	require.Len(t, results, mockDestTTL)
}
func TestParallelTracerouteShuffled(t *testing.T) {
	for seed := range 3 {
		t.Run(fmt.Sprintf("seed=%d", seed), func(t *testing.T) {
			testParallelTracerouteShuffled(t, int64(seed))
		})
	}
}

var errMock = errors.New("mock error")

func TestParallelTracerouteSendErr(t *testing.T) {
	// this test checks that TracerouteParallel returns an error if SendProbe() fails
	m := initMockDriver(t, testParams)

	hasCalled := false
	m.sendHandler = func(_ uint8) error {
		require.False(t, hasCalled, "SendProbe() called more than once")
		hasCalled = true

		return errMock
	}

	results, err := TracerouteParallel(context.Background(), m, testParams)
	require.Nil(t, results)
	require.ErrorIs(t, err, errMock)
}

func TestParallelTracerouteReceiveErr(t *testing.T) {
	// this test checks that TracerouteParallel returns an error if ReceiveProbe() fails
	m := initMockDriver(t, testParams)

	hasCalled := false
	m.receiveHandler = func() (*ProbeResponse, error) {
		require.False(t, hasCalled, "ReceiveProbe() called more than once")
		hasCalled = true

		return nil, errMock
	}

	results, err := TracerouteParallel(context.Background(), m, testParams)
	require.Nil(t, results)
	require.ErrorIs(t, err, errMock)
}

func TestParallelTracerouteTimeout(t *testing.T) {
	// this test checks that TracerouteParallel times out when it is waiting for
	// a response during the traceroute
	m := initMockDriver(t, testParams)

	totalCalls := 0
	m.receiveHandler = func() (*ProbeResponse, error) {
		totalCalls++
		return noData(testParams.PollFrequency)
	}

	start := time.Now()
	results, err := TracerouteParallel(context.Background(), m, testParams)
	require.NoError(t, err)

	// divide by 2 to give margin for error
	require.Greater(t, time.Since(start), testParams.TracerouteTimeout/2)
	// make sure it kept polling repeatedly
	require.Greater(t, totalCalls, 5)
	for _, res := range results {
		require.Nil(t, res)
	}
}

func TestParallelTracerouteMinTTL(t *testing.T) {
	// same as TestParallelTraceroute but it checks that we don't send TTL=1 when MinTTL=2

	// make a copy of testParams
	testParams := testParams
	testParams.MinTTL = 2
	m := initMockDriver(t, testParams)

	var expectedResults []*ProbeResponse
	receiveProbes := make(chan *ProbeResponse, testParams.MaxTTL)

	// expectedTTL starts at 2 in this test
	expectedTTL := uint8(2)
	m.sendHandler = func(ttl uint8) error {
		require.Equal(t, expectedTTL, ttl)
		expectedTTL++

		result := mockResult(ttl)

		if result != nil {
			expectedResults = append(expectedResults, result)
			receiveProbes <- result
			if result.IsDest {
				close(receiveProbes)
			}
		}

		return nil
	}
	m.receiveHandler = func() (*ProbeResponse, error) {
		probe, ok := <-receiveProbes
		if !ok {
			return noData(testParams.PollFrequency)
		}
		return probe, nil
	}

	results, err := TracerouteParallel(context.Background(), m, testParams)
	require.NoError(t, err)
	require.Equal(t, expectedResults, results)
	require.Len(t, results, mockDestTTL-1)
}

func TestParallelTracerouteReportsExternalCancellation(t *testing.T) {
	// this test checks that TracerouteParallel forwards a cancellation from the context
	m := initMockDriver(t, testParams)

	ctx, cancel := context.WithCancelCause(context.Background())
	// cancel it right away
	cancel(errMock)

	results, err := TracerouteParallel(ctx, m, testParams)
	require.Nil(t, results)
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, context.Cause(ctx), errMock)
}

func TestParallelTracerouteMissingHop(t *testing.T) {
	// this test simulates a missing hop at TTL=3
	m := initMockDriver(t, testParams)

	var expectedResults []*ProbeResponse
	receiveProbes := make(chan *ProbeResponse, testParams.MaxTTL)

	m.sendHandler = func(ttl uint8) error {
		result := mockResult(ttl)
		skipHop := ttl == 3

		if result != nil {
			if skipHop {
				result = nil
			}
			expectedResults = append(expectedResults, result)

			if result != nil {
				receiveProbes <- result
				if result.IsDest {
					close(receiveProbes)
				}
			}
		}

		return nil
	}
	m.receiveHandler = func() (*ProbeResponse, error) {
		probe, ok := <-receiveProbes
		if !ok {
			return noData(testParams.PollFrequency)
		}
		return probe, nil
	}

	results, err := TracerouteParallel(context.Background(), m, testParams)
	require.NoError(t, err)
	require.Equal(t, expectedResults, results)
	require.Len(t, results, mockDestTTL)
}

func TestParallelTracerouteMissingDest(t *testing.T) {
	// this test simulates not getting the destination back - it should keep sending probes until MaxTTL
	m := initMockDriver(t, testParams)

	var expectedResults []*ProbeResponse
	receiveProbes := make(chan *ProbeResponse, testParams.MaxTTL)

	m.sendHandler = func(ttl uint8) error {
		result := mockResult(ttl)
		skipHop := ttl == mockDestTTL

		if result != nil {
			if skipHop {
				result = nil
			}
			expectedResults = append(expectedResults, result)

			if !skipHop {
				receiveProbes <- result
			}
		}

		if ttl == testParams.MaxTTL {
			close(receiveProbes)
		}

		return nil
	}
	m.receiveHandler = func() (*ProbeResponse, error) {
		probe, ok := <-receiveProbes
		if !ok {
			return noData(testParams.PollFrequency)
		}
		return probe, nil
	}

	results, err := TracerouteParallel(context.Background(), m, testParams)
	require.NoError(t, err)
	require.Len(t, results, int(testParams.MaxTTL))
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

func TestParallelTracerouteProbeSanityCheck(t *testing.T) {
	// this probe checks that TracerouteParallel yells at you when it reads
	// a an invalid TTL
	m := initMockDriver(t, testParams)

	hasReceived := false
	m.receiveHandler = func() (*ProbeResponse, error) {
		require.False(t, hasReceived, "ReceiveProbe() called more than once")
		hasReceived = true
		result := mockResult(1)
		require.NotNil(t, result)
		result.TTL = 123
		return result, nil
	}

	results, err := TracerouteParallel(context.Background(), m, testParams)
	require.Nil(t, results)
	require.ErrorContains(t, err, "received an invalid TTL")
}

func TestParallelTracerouteProbeReturnValueCheck(t *testing.T) {
	// this probe checks that TracerouteParallel yells at you when you return nothing at all
	m := initMockDriver(t, testParams)

	hasReceived := false
	m.receiveHandler = func() (*ProbeResponse, error) {
		require.False(t, hasReceived, "ReceiveProbe() called more than once")
		hasReceived = true
		return nil, nil
	}

	results, err := TracerouteParallel(context.Background(), m, testParams)
	require.Nil(t, results)
	require.ErrorContains(t, err, "ReceiveProbe() returned nil without an error")
}
