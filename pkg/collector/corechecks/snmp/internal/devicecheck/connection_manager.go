// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package devicecheck

import (
	"errors"
	"fmt"
	"net"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/session"
	coresnmp "github.com/DataDog/datadog-agent/pkg/snmp"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ConnectionManager handles SNMP session creation and connection management,
// including automatic fallback to unconnected UDP sockets for multi-homed devices.
type ConnectionManager interface {
	// Connect establishes connection with automatic fallback handling.
	// Returns whether unconnected UDP fallback was enabled.
	Connect() (bool, error)

	// GetSession returns the current active session.
	// Must be called after successful Connect().
	GetSession() session.Session

	// Close closes the current session.
	Close() error
}

// snmpConnectionManager implements ConnectionManager with UDP socket fallback support.
type snmpConnectionManager struct {
	config         *checkconfig.CheckConfig
	sessionFactory session.Factory
	session        session.Session
	fallbackState  fallbackState
}

// fallbackState tracks whether we should use unconnected UDP sockets.
// Unconnected sockets accept responses from any source IP, working around devices
// that respond from IP B when queried on IP A (multi-homed devices). This prevents
// connection timeouts for devices with multiple management interfaces.
type fallbackState struct {
	connectedSocketSucceeded bool // True if standard connected socket ever worked
	fallbackTestAttempted    bool // True if we already tested unconnected fallback
	useUnconnectedSocket     bool // True if we should use unconnected UDP mode
}

// NewConnectionManager creates a new SNMP connection manager.
func NewConnectionManager(config *checkconfig.CheckConfig, sessionFactory session.Factory) ConnectionManager {
	return &snmpConnectionManager{
		config:         config,
		sessionFactory: sessionFactory,
		fallbackState:  fallbackState{},
	}
}

// Connect establishes connection with automatic fallback handling.
// Returns whether unconnected UDP fallback was enabled during this connection.
func (m *snmpConnectionManager) Connect() (bool, error) {
	// Create initial session
	sess, err := m.sessionFactory(m.config)
	if err != nil {
		return false, fmt.Errorf("failed to create session: %w", err)
	}

	// Try to connect
	err = sess.Connect()
	if err == nil {
		// Connection succeeded - now test reachability for UDP
		// This is critical because UDP Connect() doesn't actually establish a connection
		// The timeout happens during the first read operation (Get/GetNext)
		_, reachErr := sess.GetNext([]string{coresnmp.DeviceReachableGetNextOid})
		if reachErr == nil {
			// Both connect and reachability check succeeded
			if !m.fallbackState.useUnconnectedSocket {
				m.fallbackState.connectedSocketSucceeded = true
			}
			m.session = sess
			return false, nil
		}
		// Reachability check failed - this is the timeout for multi-homed devices
		err = reachErr
	}

	// Connection or reachability failed - check if we should attempt fallback
	// Convert net.Error timeouts to our typed error for consistent detection
	if isRawNetworkTimeout(err) {
		err = session.NewConnectionTimeoutError(m.config.IPAddress, err)
	}

	if !m.shouldAttemptFallback(err) {
		return false, err
	}

	// Attempt fallback
	log.Infof("[%s] Connected socket failed with timeout, testing unconnected socket fallback", m.config.IPAddress)

	fallbackSuccess, fallbackSess := m.testFallback()
	if !fallbackSuccess {
		log.Infof("[%s] Unconnected socket test failed, keeping connected mode", m.config.IPAddress)
		return false, err
	}

	// Fallback test succeeded - switch to unconnected mode
	log.Infof("[%s] Unconnected socket test succeeded, switching to unconnected mode", m.config.IPAddress)
	m.fallbackState.useUnconnectedSocket = true

	// Update config and recreate session for normal use
	m.config.UseUnconnectedUDPSocket = true
	newSess, createErr := m.sessionFactory(m.config)
	if createErr != nil {
		return false, fmt.Errorf("failed to recreate session with unconnected socket for %s: %w", m.config.IPAddress, createErr)
	}

	// Try connecting with new session
	if connectErr := newSess.Connect(); connectErr != nil {
		return false, connectErr
	}

	m.session = newSess
	return true, nil
}

// GetSession returns the current active session.
// Must be called after successful Connect().
func (m *snmpConnectionManager) GetSession() session.Session {
	return m.session
}

// Close closes the current session.
func (m *snmpConnectionManager) Close() error {
	if m.session != nil {
		return m.session.Close()
	}
	return nil
}

// shouldAttemptFallback determines if fallback should be attempted
func (m *snmpConnectionManager) shouldAttemptFallback(err error) bool {
	if m.fallbackState.useUnconnectedSocket {
		return false // Already using unconnected
	}
	if m.fallbackState.fallbackTestAttempted {
		return false // Already tested
	}
	if m.fallbackState.connectedSocketSucceeded {
		return false // Connected socket worked before
	}
	if !isTimeoutError(err) {
		return false // Not a timeout error
	}
	return true
}

// testFallback tests if unconnected socket works.
// Returns (success bool, test session for cleanup).
func (m *snmpConnectionManager) testFallback() (bool, session.Session) {
	m.fallbackState.fallbackTestAttempted = true

	// Create test session with unconnected socket
	testConfig := m.config.Copy()
	testConfig.UseUnconnectedUDPSocket = true

	testSession, err := m.sessionFactory(testConfig)
	if err != nil {
		log.Debugf("[%s] Fallback test: failed to create session: %s", m.config.IPAddress, err)
		return false, nil
	}
	defer func() {
		if closeErr := testSession.Close(); closeErr != nil {
			log.Debugf("[%s] Fallback test: failed to close session: %s", m.config.IPAddress, closeErr)
		}
	}()

	if err := testSession.Connect(); err != nil {
		log.Debugf("[%s] Fallback test: connect failed: %s", m.config.IPAddress, err)
		return false, testSession
	}

	_, err = testSession.GetNext([]string{coresnmp.DeviceReachableGetNextOid})
	if err != nil {
		log.Debugf("[%s] Fallback test: GetNext failed: %s", m.config.IPAddress, err)
		return false, testSession
	}

	return true, testSession
}

// isTimeoutError checks if error is a connection timeout using type assertion
// This is robust and doesn't rely on fragile string matching
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}

	// Check if it's our typed timeout error from the session package
	var timeoutErr *session.ConnectionTimeoutError
	return errors.As(err, &timeoutErr)
}

// isRawNetworkTimeout checks if error is a raw net.Error timeout (not yet wrapped in ConnectionTimeoutError)
// This is used to detect timeouts from Get/GetNext operations that aren't wrapped by the session layer
func isRawNetworkTimeout(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return false
}
