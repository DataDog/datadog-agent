// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package devicecheck

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/session"
	coresnmp "github.com/DataDog/datadog-agent/pkg/snmp"
)

// mockSession implements session.Session interface for testing
type mockSession struct {
	mock.Mock
}

func (m *mockSession) Connect() error {
	args := m.Called()
	return args.Error(0)
}

func (m *mockSession) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *mockSession) Get(oids []string) (*gosnmp.SnmpPacket, error) {
	args := m.Called(oids)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*gosnmp.SnmpPacket), args.Error(1)
}

func (m *mockSession) GetBulk(oids []string, bulkMaxRepetitions uint32) (*gosnmp.SnmpPacket, error) {
	args := m.Called(oids, bulkMaxRepetitions)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*gosnmp.SnmpPacket), args.Error(1)
}

func (m *mockSession) GetNext(oids []string) (*gosnmp.SnmpPacket, error) {
	args := m.Called(oids)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*gosnmp.SnmpPacket), args.Error(1)
}

func (m *mockSession) GetSnmpGetCount() uint32 {
	args := m.Called()
	return args.Get(0).(uint32)
}

func (m *mockSession) GetSnmpGetBulkCount() uint32 {
	args := m.Called()
	return args.Get(0).(uint32)
}

func (m *mockSession) GetSnmpGetNextCount() uint32 {
	args := m.Called()
	return args.Get(0).(uint32)
}

func (m *mockSession) GetVersion() gosnmp.SnmpVersion {
	args := m.Called()
	return args.Get(0).(gosnmp.SnmpVersion)
}

func (m *mockSession) IsUnconnectedUDP() bool {
	args := m.Called()
	return args.Bool(0)
}

// TestConnectionManager_ConnectSuccess tests successful connection
func TestConnectionManager_ConnectSuccess(t *testing.T) {
	config := &checkconfig.CheckConfig{
		IPAddress: "10.0.0.1",
	}

	mainSess := new(mockSession)
	mainSess.On("Connect").Return(nil).Once()
	mainSess.On("GetNext", []string{coresnmp.DeviceReachableGetNextOid}).Return(&gosnmp.SnmpPacket{}, nil).Once()

	sessionFactory := func(cfg *checkconfig.CheckConfig) (session.Session, error) {
		return mainSess, nil
	}

	connMgr := NewConnectionManager(config, sessionFactory)
	sess, err := connMgr.Connect()

	assert.NoError(t, err)
	assert.Equal(t, mainSess, sess)

	gotSess, err := connMgr.GetSession()
	assert.NoError(t, err)
	assert.Equal(t, mainSess, gotSess)

	mainSess.AssertExpectations(t)
}

// TestConnectionManager_TimeoutTriggersFallback tests that timeout triggers fallback
func TestConnectionManager_TimeoutTriggersFallback(t *testing.T) {
	config := &checkconfig.CheckConfig{
		IPAddress: "10.0.0.1",
	}

	mainSess := new(mockSession)
	testSess := new(mockSession)
	newSess := new(mockSession)

	// Main session fails with timeout on reachability check
	mainSess.On("Connect").Return(nil).Once()
	mainSess.On("GetNext", []string{coresnmp.DeviceReachableGetNextOid}).
		Return(nil, session.NewConnectionTimeoutError("10.0.0.1", errors.New("timeout"))).Once()

	// Test session with unconnected socket succeeds
	testSess.On("Connect").Return(nil).Once()
	testSess.On("GetNext", []string{coresnmp.DeviceReachableGetNextOid}).Return(&gosnmp.SnmpPacket{}, nil).Once()
	testSess.On("Close").Return(nil).Once()

	// New session with unconnected socket for actual use
	newSess.On("Connect").Return(nil).Once()

	callCount := 0
	sessionFactory := func(cfg *checkconfig.CheckConfig) (session.Session, error) {
		callCount++
		if callCount == 1 {
			// First call - main session
			return mainSess, nil
		} else if callCount == 2 {
			// Second call - test session (with UseUnconnectedUDPSocket = true)
			assert.True(t, cfg.UseUnconnectedUDPSocket, "Test session should have UseUnconnectedUDPSocket enabled")
			return testSess, nil
		} else {
			// Third call - new session for actual use
			assert.True(t, cfg.UseUnconnectedUDPSocket, "New session should have UseUnconnectedUDPSocket enabled")
			return newSess, nil
		}
	}

	connMgr := NewConnectionManager(config, sessionFactory)
	sess, err := connMgr.Connect()

	assert.NoError(t, err)
	assert.Equal(t, newSess, sess)
	assert.True(t, config.UseUnconnectedUDPSocket, "Config should be updated")

	gotSess, err := connMgr.GetSession()
	assert.NoError(t, err)
	assert.Equal(t, newSess, gotSess)

	mainSess.AssertExpectations(t)
	testSess.AssertExpectations(t)
	newSess.AssertExpectations(t)
}

// TestConnectionManager_NonTimeoutError tests that non-timeout errors don't trigger fallback
func TestConnectionManager_NonTimeoutError(t *testing.T) {
	config := &checkconfig.CheckConfig{
		IPAddress: "10.0.0.1",
	}

	mainSess := new(mockSession)
	mainSess.On("Connect").Return(nil).Once()
	mainSess.On("GetNext", []string{coresnmp.DeviceReachableGetNextOid}).
		Return(nil, errors.New("authentication failed")).Once()

	sessionFactory := func(cfg *checkconfig.CheckConfig) (session.Session, error) {
		return mainSess, nil
	}

	connMgr := NewConnectionManager(config, sessionFactory)
	sess, err := connMgr.Connect()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
	assert.Nil(t, sess)

	mainSess.AssertExpectations(t)
}

// TestConnectionManager_FallbackTestFails tests fallback failure
func TestConnectionManager_FallbackTestFails(t *testing.T) {
	config := &checkconfig.CheckConfig{
		IPAddress: "10.0.0.1",
	}

	mainSess := new(mockSession)
	testSess := new(mockSession)

	// Main session fails with timeout
	mainSess.On("Connect").Return(nil).Once()
	mainSess.On("GetNext", []string{coresnmp.DeviceReachableGetNextOid}).
		Return(nil, session.NewConnectionTimeoutError("10.0.0.1", errors.New("timeout"))).Once()

	// Test session fails
	testSess.On("Connect").Return(nil).Once()
	testSess.On("GetNext", []string{coresnmp.DeviceReachableGetNextOid}).
		Return(nil, errors.New("still fails")).Once()
	testSess.On("Close").Return(nil).Once()

	callCount := 0
	sessionFactory := func(cfg *checkconfig.CheckConfig) (session.Session, error) {
		callCount++
		if callCount == 1 {
			return mainSess, nil
		}
		return testSess, nil
	}

	connMgr := NewConnectionManager(config, sessionFactory)
	sess, err := connMgr.Connect()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
	assert.Nil(t, sess)

	mainSess.AssertExpectations(t)
	testSess.AssertExpectations(t)
}

// TestConnectionManager_Close tests connection closing
func TestConnectionManager_Close(t *testing.T) {
	config := &checkconfig.CheckConfig{
		IPAddress: "10.0.0.1",
	}

	sess := new(mockSession)
	sess.On("Connect").Return(nil).Once()
	sess.On("GetNext", []string{coresnmp.DeviceReachableGetNextOid}).Return(&gosnmp.SnmpPacket{}, nil).Once()
	sess.On("Close").Return(nil).Once()

	sessionFactory := func(cfg *checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}

	connMgr := NewConnectionManager(config, sessionFactory)
	gotSess, err := connMgr.Connect()
	assert.NoError(t, err)
	assert.Equal(t, sess, gotSess)

	err = connMgr.Close()
	assert.NoError(t, err)

	sess.AssertExpectations(t)
}

// TestIsTimeoutError tests timeout error detection
func TestIsTimeoutError(t *testing.T) {
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
			name:     "ConnectionTimeoutError",
			err:      session.NewConnectionTimeoutError("10.0.0.1", errors.New("timeout")),
			expected: true,
		},
		{
			name:     "wrapped ConnectionTimeoutError",
			err:      fmt.Errorf("wrapped: %w", session.NewConnectionTimeoutError("10.0.0.1", errors.New("timeout"))),
			expected: true,
		},
		{
			name:     "generic error",
			err:      errors.New("some other error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTimeoutError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
