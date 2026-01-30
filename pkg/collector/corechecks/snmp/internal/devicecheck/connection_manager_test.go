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

// mockTimeoutError implements net.Error for testing timeout scenarios
type mockTimeoutError struct {
	msg string
}

func (e *mockTimeoutError) Error() string   { return e.msg }
func (e *mockTimeoutError) Timeout() bool   { return true }
func (e *mockTimeoutError) Temporary() bool { return false }

func newMockTimeoutError(msg string) error {
	return &mockTimeoutError{msg: msg}
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
	sess, reachable, err := connMgr.Connect()

	assert.NoError(t, err)
	assert.True(t, reachable)
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

	connectedSess := new(mockSession)
	unconnectedSess := new(mockSession)

	// Connected session fails with timeout on reachability check
	connectedSess.On("Connect").Return(nil).Once()
	connectedSess.On("GetNext", []string{coresnmp.DeviceReachableGetNextOid}).
		Return(nil, newMockTimeoutError("timeout")).Once()
	connectedSess.On("Close").Return(nil).Once()

	// Unconnected session succeeds
	unconnectedSess.On("Connect").Return(nil).Once()
	unconnectedSess.On("GetNext", []string{coresnmp.DeviceReachableGetNextOid}).
		Return(&gosnmp.SnmpPacket{}, nil).Once()

	callCount := 0
	sessionFactory := func(cfg *checkconfig.CheckConfig) (session.Session, error) {
		callCount++
		if callCount == 1 {
			// First call - connected session
			assert.False(t, cfg.UseUnconnectedUDPSocket, "First session should use connected mode")
			return connectedSess, nil
		}
		// Second call - unconnected session
		assert.True(t, cfg.UseUnconnectedUDPSocket, "Second session should have UseUnconnectedUDPSocket enabled")
		return unconnectedSess, nil
	}

	connMgr := NewConnectionManager(config, sessionFactory)
	sess, reachable, err := connMgr.Connect()

	assert.NoError(t, err)
	assert.True(t, reachable)
	assert.Equal(t, unconnectedSess, sess)
	// Note: Original config should NOT be modified (ConnectionManager makes a copy)

	gotSess, err := connMgr.GetSession()
	assert.NoError(t, err)
	assert.Equal(t, unconnectedSess, gotSess)

	connectedSess.AssertExpectations(t)
	unconnectedSess.AssertExpectations(t)
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
	sess, reachable, err := connMgr.Connect()

	assert.NoError(t, err)
	assert.False(t, reachable)
	// Session should be returned even on reachability failure so caller can continue
	assert.NotNil(t, sess)
	assert.Equal(t, mainSess, sess)

	mainSess.AssertExpectations(t)
}

// TestConnectionManager_FallbackTestFails tests fallback when unconnected is also unreachable
func TestConnectionManager_FallbackTestFails(t *testing.T) {
	config := &checkconfig.CheckConfig{
		IPAddress: "10.0.0.1",
	}

	connectedSess := new(mockSession)
	unconnectedSess := new(mockSession)

	// Connected session fails with timeout
	connectedSess.On("Connect").Return(nil).Once()
	connectedSess.On("GetNext", []string{coresnmp.DeviceReachableGetNextOid}).
		Return(nil, newMockTimeoutError("timeout")).Once()
	connectedSess.On("Close").Return(nil).Once()

	// Unconnected session connects but is also unreachable
	unconnectedSess.On("Connect").Return(nil).Once()
	unconnectedSess.On("GetNext", []string{coresnmp.DeviceReachableGetNextOid}).
		Return(nil, errors.New("still fails")).Once()

	callCount := 0
	sessionFactory := func(cfg *checkconfig.CheckConfig) (session.Session, error) {
		callCount++
		if callCount == 1 {
			return connectedSess, nil
		}
		return unconnectedSess, nil
	}

	connMgr := NewConnectionManager(config, sessionFactory)
	sess, reachable, err := connMgr.Connect()

	assert.NoError(t, err)
	assert.False(t, reachable)
	// With new logic, we use unconnected session even if unreachable
	assert.NotNil(t, sess)
	assert.Equal(t, unconnectedSess, sess)

	connectedSess.AssertExpectations(t)
	unconnectedSess.AssertExpectations(t)
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
	gotSess, reachable, err := connMgr.Connect()
	assert.NoError(t, err)
	assert.True(t, reachable)
	assert.Equal(t, sess, gotSess)

	err = connMgr.Close()
	assert.NoError(t, err)

	sess.AssertExpectations(t)
}

// TestConnectionManager_GetSessionAfterClose tests that GetSession returns error after Close
func TestConnectionManager_GetSessionAfterClose(t *testing.T) {
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

	// Connect should succeed
	gotSess, reachable, err := connMgr.Connect()
	assert.NoError(t, err)
	assert.True(t, reachable)
	assert.Equal(t, sess, gotSess)

	// GetSession should work before Close
	gotSess, err = connMgr.GetSession()
	assert.NoError(t, err)
	assert.Equal(t, sess, gotSess)

	// Close the session
	err = connMgr.Close()
	assert.NoError(t, err)

	// GetSession should now return an error
	gotSess, err = connMgr.GetSession()
	assert.Error(t, err)
	assert.Nil(t, gotSess)
	assert.Contains(t, err.Error(), "session not initialized")

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
			name:     "network timeout error",
			err:      newMockTimeoutError("timeout"),
			expected: true,
		},
		{
			name:     "wrapped network timeout",
			err:      fmt.Errorf("wrapped: %w", newMockTimeoutError("timeout")),
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
