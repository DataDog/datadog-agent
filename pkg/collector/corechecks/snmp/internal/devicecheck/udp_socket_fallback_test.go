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

// TestConnectionTimeoutError tests the ConnectionTimeoutError type
func TestConnectionTimeoutError(t *testing.T) {
	underlyingErr := errors.New("i/o timeout")
	timeoutErr := session.NewConnectionTimeoutError("10.0.0.1", underlyingErr)

	// Test Error() message
	assert.Contains(t, timeoutErr.Error(), "connection timeout to 10.0.0.1")
	assert.Contains(t, timeoutErr.Error(), "i/o timeout")

	// Test Unwrap()
	assert.Equal(t, underlyingErr, timeoutErr.Unwrap())

	// Test that it can be detected with errors.As()
	var ctErr *session.ConnectionTimeoutError
	assert.True(t, errors.As(timeoutErr, &ctErr))
}

// TestIsTimeoutError tests the isTimeoutError function
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
		{
			name:     "connection refused",
			err:      errors.New("connection refused"),
			expected: false,
		},
		{
			name:     "authentication error",
			err:      errors.New("authentication failed"),
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

// TestShouldAttemptFallback tests the shouldAttemptFallback logic
func TestShouldAttemptFallback(t *testing.T) {
	tests := []struct {
		name                     string
		useUnconnectedSocket     bool
		fallbackTestAttempted    bool
		connectedSocketSucceeded bool
		err                      error
		expected                 bool
	}{
		{
			name:                     "should attempt - all conditions met",
			useUnconnectedSocket:     false,
			fallbackTestAttempted:    false,
			connectedSocketSucceeded: false,
			err:                      session.NewConnectionTimeoutError("10.0.0.1", errors.New("timeout")),
			expected:                 true,
		},
		{
			name:                     "should not attempt - already using unconnected",
			useUnconnectedSocket:     true,
			fallbackTestAttempted:    false,
			connectedSocketSucceeded: false,
			err:                      session.NewConnectionTimeoutError("10.0.0.1", errors.New("timeout")),
			expected:                 false,
		},
		{
			name:                     "should not attempt - already tested",
			useUnconnectedSocket:     false,
			fallbackTestAttempted:    true,
			connectedSocketSucceeded: false,
			err:                      session.NewConnectionTimeoutError("10.0.0.1", errors.New("timeout")),
			expected:                 false,
		},
		{
			name:                     "should not attempt - connected succeeded before",
			useUnconnectedSocket:     false,
			fallbackTestAttempted:    false,
			connectedSocketSucceeded: true,
			err:                      session.NewConnectionTimeoutError("10.0.0.1", errors.New("timeout")),
			expected:                 false,
		},
		{
			name:                     "should not attempt - not a timeout error",
			useUnconnectedSocket:     false,
			fallbackTestAttempted:    false,
			connectedSocketSucceeded: false,
			err:                      errors.New("authentication failed"),
			expected:                 false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &checkconfig.CheckConfig{IPAddress: "10.0.0.1"}
			manager := &udpSocketFallbackManager{
				state: udpSocketFallbackState{
					useUnconnectedSocket:     tt.useUnconnectedSocket,
					fallbackTestAttempted:    tt.fallbackTestAttempted,
					connectedSocketSucceeded: tt.connectedSocketSucceeded,
				},
				config: config,
			}

			result := manager.shouldAttemptFallback(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestTestFallback tests the testFallback method
func TestTestFallback(t *testing.T) {
	tests := []struct {
		name           string
		setupMock      func(*mockSession)
		expectedResult bool
	}{
		{
			name: "fallback test succeeds",
			setupMock: func(sess *mockSession) {
				sess.On("Connect").Return(nil).Once()
				sess.On("GetNext", []string{coresnmp.DeviceReachableGetNextOid}).Return(&gosnmp.SnmpPacket{}, nil).Once()
				sess.On("Close").Return(nil).Once()
			},
			expectedResult: true,
		},
		{
			name: "fallback test fails - connect fails",
			setupMock: func(sess *mockSession) {
				sess.On("Connect").Return(errors.New("connection failed")).Once()
				sess.On("Close").Return(nil).Once()
			},
			expectedResult: false,
		},
		{
			name: "fallback test fails - GetNext fails",
			setupMock: func(sess *mockSession) {
				sess.On("Connect").Return(nil).Once()
				sess.On("GetNext", []string{coresnmp.DeviceReachableGetNextOid}).Return(nil, errors.New("getnext failed")).Once()
				sess.On("Close").Return(nil).Once()
			},
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testSess := new(mockSession)
			tt.setupMock(testSess)

			config := &checkconfig.CheckConfig{
				IPAddress: "10.0.0.1",
			}

			sessionFactory := func(cfg *checkconfig.CheckConfig) (session.Session, error) {
				// Verify that UseUnconnectedUDPSocket was set
				assert.True(t, cfg.UseUnconnectedUDPSocket)
				return testSess, nil
			}

			manager := &udpSocketFallbackManager{
				state:          udpSocketFallbackState{},
				config:         config,
				sessionFactory: sessionFactory,
			}

			result := manager.testFallback()
			assert.Equal(t, tt.expectedResult, result)
			assert.True(t, manager.state.fallbackTestAttempted)

			testSess.AssertExpectations(t)
		})
	}
}

// TestTestFallback_SessionFactoryError tests testFallback when session creation fails
func TestTestFallback_SessionFactoryError(t *testing.T) {
	config := &checkconfig.CheckConfig{
		IPAddress: "10.0.0.1",
	}

	sessionFactory := func(cfg *checkconfig.CheckConfig) (session.Session, error) {
		return nil, errors.New("failed to create session")
	}

	manager := &udpSocketFallbackManager{
		state:          udpSocketFallbackState{},
		config:         config,
		sessionFactory: sessionFactory,
	}

	result := manager.testFallback()
	assert.False(t, result)
	assert.True(t, manager.state.fallbackTestAttempted)
}

// TestConnectWithFallback tests the complete connectWithFallback flow
func TestConnectWithFallback(t *testing.T) {
	tests := []struct {
		name                   string
		initialState           udpSocketFallbackState
		sessionConnectError    error
		testFallbackSucceeds   bool
		newSessionConnectError error
		expectedFallbackUsed   bool
		expectError            bool
	}{
		{
			name: "connect succeeds immediately - no fallback needed",
			initialState: udpSocketFallbackState{
				useUnconnectedSocket:     false,
				fallbackTestAttempted:    false,
				connectedSocketSucceeded: false,
			},
			sessionConnectError:  nil,
			expectedFallbackUsed: false,
			expectError:          false,
		},
		{
			name: "timeout triggers fallback - fallback succeeds",
			initialState: udpSocketFallbackState{
				useUnconnectedSocket:     false,
				fallbackTestAttempted:    false,
				connectedSocketSucceeded: false,
			},
			sessionConnectError:    session.NewConnectionTimeoutError("10.0.0.1", errors.New("timeout")),
			testFallbackSucceeds:   true,
			newSessionConnectError: nil,
			expectedFallbackUsed:   true,
			expectError:            false,
		},
		{
			name: "timeout triggers fallback - fallback test fails",
			initialState: udpSocketFallbackState{
				useUnconnectedSocket:     false,
				fallbackTestAttempted:    false,
				connectedSocketSucceeded: false,
			},
			sessionConnectError:  session.NewConnectionTimeoutError("10.0.0.1", errors.New("timeout")),
			testFallbackSucceeds: false,
			expectedFallbackUsed: false,
			expectError:          true,
		},
		{
			name: "non-timeout error - no fallback",
			initialState: udpSocketFallbackState{
				useUnconnectedSocket:     false,
				fallbackTestAttempted:    false,
				connectedSocketSucceeded: false,
			},
			sessionConnectError:  errors.New("authentication failed"),
			expectedFallbackUsed: false,
			expectError:          true,
		},
		{
			name: "already using unconnected socket - connects successfully",
			initialState: udpSocketFallbackState{
				useUnconnectedSocket:     true,
				fallbackTestAttempted:    true,
				connectedSocketSucceeded: false,
			},
			sessionConnectError:  nil,
			expectedFallbackUsed: false,
			expectError:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &checkconfig.CheckConfig{
				IPAddress: "10.0.0.1",
			}

			mainSess := new(mockSession)
			mainSess.On("Connect").Return(tt.sessionConnectError).Once()

			var testSess *mockSession
			var newSess *mockSession

			sessionFactory := func(cfg *checkconfig.CheckConfig) (session.Session, error) {
				if cfg.UseUnconnectedUDPSocket {
					// This is either the test session or the new session after fallback
					if testSess == nil {
						// First call is for testing
						testSess = new(mockSession)
						if tt.testFallbackSucceeds {
							testSess.On("Connect").Return(nil).Once()
							testSess.On("GetNext", []string{coresnmp.DeviceReachableGetNextOid}).Return(&gosnmp.SnmpPacket{}, nil).Once()
						} else {
							testSess.On("Connect").Return(errors.New("test failed")).Once()
						}
						testSess.On("Close").Return(nil).Once()
						return testSess, nil
					}
					// Second call is for the actual new session
					newSess = new(mockSession)
					newSess.On("Connect").Return(tt.newSessionConnectError).Once()
					return newSess, nil
				}
				return mainSess, nil
			}

			manager := &udpSocketFallbackManager{
				state:          tt.initialState,
				config:         config,
				sessionFactory: sessionFactory,
			}

			resultSess, fallbackEnabled, err := manager.connectWithFallback(mainSess)

			assert.Equal(t, tt.expectedFallbackUsed, fallbackEnabled)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Verify session is not nil
			assert.NotNil(t, resultSess)

			// Verify state updates
			if tt.expectedFallbackUsed {
				assert.True(t, manager.state.useUnconnectedSocket)
				assert.True(t, manager.state.fallbackTestAttempted)
				assert.True(t, config.UseUnconnectedUDPSocket)
			}

			mainSess.AssertExpectations(t)
			if testSess != nil {
				testSess.AssertExpectations(t)
			}
			if newSess != nil {
				newSess.AssertExpectations(t)
			}
		})
	}
}

// TestConnectWithFallback_ConnectedSocketSuccess tests that successful connected socket sets the flag
func TestConnectWithFallback_ConnectedSocketSuccess(t *testing.T) {
	config := &checkconfig.CheckConfig{
		IPAddress: "10.0.0.1",
	}

	mainSess := new(mockSession)
	mainSess.On("Connect").Return(nil).Once()

	sessionFactory := func(cfg *checkconfig.CheckConfig) (session.Session, error) {
		return mainSess, nil
	}

	manager := &udpSocketFallbackManager{
		state: udpSocketFallbackState{
			useUnconnectedSocket:     false,
			fallbackTestAttempted:    false,
			connectedSocketSucceeded: false,
		},
		config:         config,
		sessionFactory: sessionFactory,
	}

	resultSess, fallbackEnabled, err := manager.connectWithFallback(mainSess)

	assert.NoError(t, err)
	assert.False(t, fallbackEnabled)
	assert.NotNil(t, resultSess)
	assert.True(t, manager.state.connectedSocketSucceeded)

	mainSess.AssertExpectations(t)
}
