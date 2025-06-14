// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package common

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type MockDriver struct {
	t      *testing.T
	params TracerouteParams

	sentTTLs map[uint8]struct{}

	info           TracerouteDriverInfo
	sendHandler    func(ttl uint8) error
	receiveHandler func() (*ProbeResponse, error)
}

var parallelInfo = TracerouteDriverInfo{
	SupportsParallel: true,
}

func initMockDriver(t *testing.T, params TracerouteParams, info TracerouteDriverInfo) *MockDriver {
	return &MockDriver{
		t:              t,
		params:         params,
		info:           info,
		sentTTLs:       make(map[uint8]struct{}),
		sendHandler:    nil,
		receiveHandler: nil,
	}
}

func (m *MockDriver) GetDriverInfo() TracerouteDriverInfo {
	return m.info
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
		return noData(timeout)
	}
	res, err := m.receiveHandler()
	var errNoPkt *ReceiveProbeNoPktError
	if !errors.As(err, &errNoPkt) {
		m.t.Logf("read %+v, %v\n", res, err)
	}
	return res, err
}

func noData(pollFrequency time.Duration) (*ProbeResponse, error) {
	time.Sleep(pollFrequency)
	return nil, &ReceiveProbeNoPktError{Err: fmt.Errorf("testing, no data")}
}

func TestClipResultsDest(t *testing.T) {
	results := []*ProbeResponse{
		nil,
		{TTL: 1, IsDest: false},
		{TTL: 2, IsDest: false},
		{TTL: 3, IsDest: true},
		nil,
	}

	clipped := clipResults(1, results)
	require.Equal(t, results[1:4], clipped)
}

func TestClipResultsNoDest(t *testing.T) {
	results := []*ProbeResponse{
		nil,
		{TTL: 1, IsDest: false},
		{TTL: 2, IsDest: false},
		{TTL: 3, IsDest: false},
		nil,
	}

	clipped := clipResults(1, results)
	require.Equal(t, results[1:], clipped)
}

func TestClipResultsMinTTL(t *testing.T) {
	results := []*ProbeResponse{
		nil,
		nil,
		{TTL: 2, IsDest: false},
		{TTL: 3, IsDest: false},
		nil,
	}

	clipped := clipResults(2, results)
	require.Equal(t, results[2:], clipped)
}
