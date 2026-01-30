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

// ConnectionManager handles SNMP session creation and connection management.
type ConnectionManager interface {
	// Connect establishes a connection and returns the active session.
	Connect() (session.Session, error)

	// GetSession returns the current active session if initialized.
	GetSession() (session.Session, error)

	// Close closes the current session.
	Close() error
}

// snmpConnectionManager implements ConnectionManager with automatic fallback support.
type snmpConnectionManager struct {
	config         *checkconfig.CheckConfig
	sessionFactory session.Factory
	session        session.Session
	fallbackState  udpFallbackState
}

// udpFallbackState tracks whether we should use unconnected UDP sockets for multi-homed devices.
// Unconnected sockets accept responses from any source IP, working around devices that respond
// from IP B when queried on IP A, which prevents connection timeouts.
type udpFallbackState struct {
	connectedSocketSucceeded bool // True if standard connected socket ever worked
	fallbackTestAttempted    bool // True if we already tested unconnected fallback
	useUnconnectedSocket     bool // True if we should use unconnected UDP mode
}

// NewConnectionManager creates a new SNMP connection manager.
func NewConnectionManager(config *checkconfig.CheckConfig, sessionFactory session.Factory) ConnectionManager {
	return &snmpConnectionManager{
		config:         config,
		sessionFactory: sessionFactory,
		fallbackState:  udpFallbackState{},
	}
}

// Connect establishes a connection and returns the active session.
// Automatically falls back to unconnected UDP sockets for multi-homed devices.
func (m *snmpConnectionManager) Connect() (session.Session, error) {
	// Create initial session
	sess, err := m.sessionFactory(m.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Try to connect
	err = sess.Connect()
	if err == nil {
		// Connection succeeded - now test reachability
		// For UDP, Connect() doesn't establish a real connection; timeouts occur during read operations
		_, reachErr := sess.GetNext([]string{coresnmp.DeviceReachableGetNextOid})
		if reachErr == nil {
			// Both connect and reachability check succeeded
			if !m.fallbackState.useUnconnectedSocket {
				m.fallbackState.connectedSocketSucceeded = true
			}
			m.session = sess
			return sess, nil
		}
		// Reachability check failed
		err = reachErr
	}

	// Connection or reachability failed - check if we should attempt fallback
	if !m.shouldAttemptFallback(err) {
		return nil, err
	}

	// Attempt unconnected UDP fallback for multi-homed devices
	log.Infof("[%s] Connected socket failed with timeout, testing unconnected socket fallback", m.config.IPAddress)

	if !m.testFallback() {
		log.Infof("[%s] Unconnected socket test failed, keeping connected mode", m.config.IPAddress)
		return nil, err
	}

	// Fallback test succeeded - switch to unconnected mode permanently
	log.Infof("[%s] Unconnected socket test succeeded, switching to unconnected mode", m.config.IPAddress)
	m.fallbackState.useUnconnectedSocket = true

	// Update config and recreate session for normal use
	m.config.UseUnconnectedUDPSocket = true
	newSess, createErr := m.sessionFactory(m.config)
	if createErr != nil {
		return nil, fmt.Errorf("failed to recreate session with unconnected socket for %s: %w", m.config.IPAddress, createErr)
	}

	// Try connecting with new session
	if connectErr := newSess.Connect(); connectErr != nil {
		return nil, connectErr
	}

	m.session = newSess
	return newSess, nil
}

// GetSession returns the current active session.
// Returns an error if no session has been initialized via Connect().
func (m *snmpConnectionManager) GetSession() (session.Session, error) {
	if m.session == nil {
		return nil, errors.New("session not initialized - call Connect() first")
	}
	return m.session, nil
}

// Close closes the current session and clears it.
// After Close(), GetSession() will return an error until Connect() is called again.
func (m *snmpConnectionManager) Close() error {
	if m.session != nil {
		err := m.session.Close()
		m.session = nil
		return err
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

// testFallback tests if unconnected socket works for multi-homed devices.
// Returns true if the test succeeded.
func (m *snmpConnectionManager) testFallback() bool {
	m.fallbackState.fallbackTestAttempted = true

	// Create test session with unconnected socket
	testConfig := m.config.Copy()
	testConfig.UseUnconnectedUDPSocket = true

	testSession, err := m.sessionFactory(testConfig)
	if err != nil {
		log.Debugf("[%s] Fallback test: failed to create session: %s", m.config.IPAddress, err)
		return false
	}
	defer func() {
		if closeErr := testSession.Close(); closeErr != nil {
			log.Debugf("[%s] Fallback test: failed to close session: %s", m.config.IPAddress, closeErr)
		}
	}()

	if err := testSession.Connect(); err != nil {
		log.Debugf("[%s] Fallback test: connect failed: %s", m.config.IPAddress, err)
		return false
	}

	_, err = testSession.GetNext([]string{coresnmp.DeviceReachableGetNextOid})
	if err != nil {
		log.Debugf("[%s] Fallback test: GetNext failed: %s", m.config.IPAddress, err)
		return false
	}

	return true
}

// isTimeoutError checks if error is a network timeout.
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}

	return false
}
