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

func newMockTimeoutError() error {
	return errors.New(".* timeout .*")
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

	sessionFactory := func(_ *checkconfig.CheckConfig) (session.Session, error) {
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
		Return(nil, newMockTimeoutError()).Once()
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

	sessionFactory := func(_ *checkconfig.CheckConfig) (session.Session, error) {
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
		Return(nil, newMockTimeoutError()).Once()

	// Unconnected session connects but is also unreachable
	unconnectedSess.On("Connect").Return(nil).Once()
	unconnectedSess.On("GetNext", []string{coresnmp.DeviceReachableGetNextOid}).
		Return(nil, errors.New("still fails")).Once()
	unconnectedSess.On("Close").Return(nil).Once()

	callCount := 0
	sessionFactory := func(_ *checkconfig.CheckConfig) (session.Session, error) {
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
	// When unconnected is also unreachable, fall back to connected session
	assert.NotNil(t, sess)
	assert.Equal(t, connectedSess, sess)

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

	sessionFactory := func(_ *checkconfig.CheckConfig) (session.Session, error) {
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

	sessionFactory := func(_ *checkconfig.CheckConfig) (session.Session, error) {
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

// TestConnectionManager_SubsequentConnectUsesConnectedMode tests that after first success,
// subsequent Connect() calls use connected mode without re-testing fallback
func TestConnectionManager_SubsequentConnectUsesConnectedMode(t *testing.T) {
	config := &checkconfig.CheckConfig{
		IPAddress: "10.0.0.1",
	}

	firstSess := new(mockSession)
	secondSess := new(mockSession)

	// First session succeeds
	firstSess.On("Connect").Return(nil).Once()
	firstSess.On("GetNext", []string{coresnmp.DeviceReachableGetNextOid}).Return(&gosnmp.SnmpPacket{}, nil).Once()
	firstSess.On("Close").Return(nil).Once()

	// Second session also succeeds (should use connected mode)
	secondSess.On("Connect").Return(nil).Once()
	secondSess.On("GetNext", []string{coresnmp.DeviceReachableGetNextOid}).Return(&gosnmp.SnmpPacket{}, nil).Once()

	callCount := 0
	sessionFactory := func(cfg *checkconfig.CheckConfig) (session.Session, error) {
		callCount++
		// Both calls should use connected mode (UseUnconnectedUDPSocket = false)
		assert.False(t, cfg.UseUnconnectedUDPSocket, "Call %d should use connected mode", callCount)
		if callCount == 1 {
			return firstSess, nil
		}
		return secondSess, nil
	}

	connMgr := NewConnectionManager(config, sessionFactory)

	// First connect
	sess, reachable, err := connMgr.Connect()
	assert.NoError(t, err)
	assert.True(t, reachable)
	assert.Equal(t, firstSess, sess)

	// Close and reconnect
	err = connMgr.Close()
	assert.NoError(t, err)

	// Second connect - should use connected mode, not test fallback
	sess, reachable, err = connMgr.Connect()
	assert.NoError(t, err)
	assert.True(t, reachable)
	assert.Equal(t, secondSess, sess)

	firstSess.AssertExpectations(t)
	secondSess.AssertExpectations(t)
}

// TestConnectionManager_SubsequentConnectUsesUnconnectedMode tests that after fallback is chosen,
// subsequent Connect() calls use unconnected mode
func TestConnectionManager_SubsequentConnectUsesUnconnectedMode(t *testing.T) {
	config := &checkconfig.CheckConfig{
		IPAddress: "10.0.0.1",
	}

	connectedSess := new(mockSession)
	unconnectedSess1 := new(mockSession)
	unconnectedSess2 := new(mockSession)

	// First: connected times out
	connectedSess.On("Connect").Return(nil).Once()
	connectedSess.On("GetNext", []string{coresnmp.DeviceReachableGetNextOid}).
		Return(nil, newMockTimeoutError()).Once()
	connectedSess.On("Close").Return(nil).Once()

	// First: unconnected succeeds
	unconnectedSess1.On("Connect").Return(nil).Once()
	unconnectedSess1.On("GetNext", []string{coresnmp.DeviceReachableGetNextOid}).
		Return(&gosnmp.SnmpPacket{}, nil).Once()
	unconnectedSess1.On("Close").Return(nil).Once()

	// Second connect: should directly use unconnected mode
	unconnectedSess2.On("Connect").Return(nil).Once()
	unconnectedSess2.On("GetNext", []string{coresnmp.DeviceReachableGetNextOid}).
		Return(&gosnmp.SnmpPacket{}, nil).Once()

	callCount := 0
	sessionFactory := func(cfg *checkconfig.CheckConfig) (session.Session, error) {
		callCount++
		switch callCount {
		case 1:
			assert.False(t, cfg.UseUnconnectedUDPSocket, "First call should be connected")
			return connectedSess, nil
		case 2:
			assert.True(t, cfg.UseUnconnectedUDPSocket, "Second call should be unconnected (fallback test)")
			return unconnectedSess1, nil
		case 3:
			assert.True(t, cfg.UseUnconnectedUDPSocket, "Third call should be unconnected (mode decided)")
			return unconnectedSess2, nil
		}
		t.Fatalf("Unexpected call count: %d", callCount)
		return nil, nil
	}

	connMgr := NewConnectionManager(config, sessionFactory)

	// First connect - triggers fallback
	sess, reachable, err := connMgr.Connect()
	assert.NoError(t, err)
	assert.True(t, reachable)
	assert.Equal(t, unconnectedSess1, sess)

	// Close and reconnect
	err = connMgr.Close()
	assert.NoError(t, err)

	// Second connect - should use unconnected mode directly
	sess, reachable, err = connMgr.Connect()
	assert.NoError(t, err)
	assert.True(t, reachable)
	assert.Equal(t, unconnectedSess2, sess)

	connectedSess.AssertExpectations(t)
	unconnectedSess1.AssertExpectations(t)
	unconnectedSess2.AssertExpectations(t)
}

// TestConnectionManager_SessionFactoryError tests that factory errors are propagated
func TestConnectionManager_SessionFactoryError(t *testing.T) {
	config := &checkconfig.CheckConfig{
		IPAddress: "10.0.0.1",
	}

	factoryErr := errors.New("failed to create session")
	sessionFactory := func(_ *checkconfig.CheckConfig) (session.Session, error) {
		return nil, factoryErr
	}

	connMgr := NewConnectionManager(config, sessionFactory)
	sess, reachable, err := connMgr.Connect()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create session")
	assert.False(t, reachable)
	assert.Nil(t, sess)
}

// TestConnectionManager_SessionConnectError tests that Connect() errors are propagated
func TestConnectionManager_SessionConnectError(t *testing.T) {
	config := &checkconfig.CheckConfig{
		IPAddress: "10.0.0.1",
	}

	mainSess := new(mockSession)
	mainSess.On("Connect").Return(errors.New("connection refused")).Once()

	sessionFactory := func(_ *checkconfig.CheckConfig) (session.Session, error) {
		return mainSess, nil
	}

	connMgr := NewConnectionManager(config, sessionFactory)
	sess, reachable, err := connMgr.Connect()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
	assert.False(t, reachable)
	assert.Nil(t, sess)

	mainSess.AssertExpectations(t)
}

// TestConnectionManager_UnconnectedSessionCreationFails tests that when unconnected session
// creation fails during fallback, we stick with the connected session
func TestConnectionManager_UnconnectedSessionCreationFails(t *testing.T) {
	config := &checkconfig.CheckConfig{
		IPAddress: "10.0.0.1",
	}

	connectedSess := new(mockSession)

	// Connected session times out
	connectedSess.On("Connect").Return(nil).Once()
	connectedSess.On("GetNext", []string{coresnmp.DeviceReachableGetNextOid}).
		Return(nil, newMockTimeoutError()).Once()

	callCount := 0
	sessionFactory := func(_ *checkconfig.CheckConfig) (session.Session, error) {
		callCount++
		if callCount == 1 {
			return connectedSess, nil
		}
		// Unconnected session creation fails
		return nil, errors.New("failed to create unconnected session")
	}

	connMgr := NewConnectionManager(config, sessionFactory)
	sess, reachable, err := connMgr.Connect()

	// Should return connected session even though it's unreachable
	assert.NoError(t, err)
	assert.False(t, reachable)
	assert.Equal(t, connectedSess, sess)

	connectedSess.AssertExpectations(t)
}

// TestConnectionManager_GetSessionBeforeConnect tests GetSession before any Connect call
func TestConnectionManager_GetSessionBeforeConnect(t *testing.T) {
	config := &checkconfig.CheckConfig{
		IPAddress: "10.0.0.1",
	}

	sessionFactory := func(_ *checkconfig.CheckConfig) (session.Session, error) {
		return nil, nil
	}

	connMgr := NewConnectionManager(config, sessionFactory)
	sess, err := connMgr.GetSession()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session not initialized")
	assert.Nil(t, sess)
}

// TestConnectionManager_CloseWithoutSession tests Close when no session exists
func TestConnectionManager_CloseWithoutSession(t *testing.T) {
	config := &checkconfig.CheckConfig{
		IPAddress: "10.0.0.1",
	}

	sessionFactory := func(_ *checkconfig.CheckConfig) (session.Session, error) {
		return nil, nil
	}

	connMgr := NewConnectionManager(config, sessionFactory)
	err := connMgr.Close()

	assert.NoError(t, err)
}

// TestConnectionManager_ReconnectAfterClose tests that Connect works after Close
func TestConnectionManager_ReconnectAfterClose(t *testing.T) {
	config := &checkconfig.CheckConfig{
		IPAddress: "10.0.0.1",
	}

	sess1 := new(mockSession)
	sess2 := new(mockSession)

	sess1.On("Connect").Return(nil).Once()
	sess1.On("GetNext", []string{coresnmp.DeviceReachableGetNextOid}).Return(&gosnmp.SnmpPacket{}, nil).Once()
	sess1.On("Close").Return(nil).Once()

	sess2.On("Connect").Return(nil).Once()
	sess2.On("GetNext", []string{coresnmp.DeviceReachableGetNextOid}).Return(&gosnmp.SnmpPacket{}, nil).Once()

	callCount := 0
	sessionFactory := func(_ *checkconfig.CheckConfig) (session.Session, error) {
		callCount++
		if callCount == 1 {
			return sess1, nil
		}
		return sess2, nil
	}

	connMgr := NewConnectionManager(config, sessionFactory)

	// First connect
	gotSess, reachable, err := connMgr.Connect()
	assert.NoError(t, err)
	assert.True(t, reachable)
	assert.Equal(t, sess1, gotSess)

	// Close
	err = connMgr.Close()
	assert.NoError(t, err)

	// Verify session is cleared
	gotSess, err = connMgr.GetSession()
	assert.Error(t, err)
	assert.Nil(t, gotSess)

	// Reconnect
	gotSess, reachable, err = connMgr.Connect()
	assert.NoError(t, err)
	assert.True(t, reachable)
	assert.Equal(t, sess2, gotSess)

	sess1.AssertExpectations(t)
	sess2.AssertExpectations(t)
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
			err:      newMockTimeoutError(),
			expected: true,
		},
		{
			name:     "wrapped network timeout",
			err:      fmt.Errorf("wrapped: %w", newMockTimeoutError()),
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
