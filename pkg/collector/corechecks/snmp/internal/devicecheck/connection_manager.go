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
	// Connect establishes a connection and returns (session, deviceReachable, error).
	// Returns:
	//   - (nil, false, error): Cannot connect at all (socket failure)
	//   - (session, false, nil): Connected but device is unreachable
	//   - (session, true, nil): Connected and device is reachable
	Connect() (session.Session, bool, error)

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
		config:         config.Copy(), // Make a copy to avoid mutating caller's config
		sessionFactory: sessionFactory,
		fallbackState:  udpFallbackState{},
	}
}

// Connect establishes a connection and returns (session, deviceReachable, error).
// Automatically falls back to unconnected UDP sockets for multi-homed devices.
//
// On first call with a timeout:
//   - Tries connected socket, times out
//   - Tests unconnected socket once
//   - Permanently decides which mode to use based on test result
//
// On subsequent calls:
//   - Uses the previously decided mode (no more testing)
func (m *snmpConnectionManager) Connect() (session.Session, bool, error) {
	// If we've permanently decided to use unconnected mode, use it
	if m.fallbackState.useUnconnectedSocket {
		return m.connectWithUnconnectedSocket()
	}

	// Try connected socket
	sess, reachable, err := m.tryConnect(m.config)

	// Check if connected socket works perfectly
	if err == nil && reachable {
		// Connected socket works - remember this for future calls
		m.fallbackState.connectedSocketSucceeded = true
		m.session = sess
		return sess, true, nil
	}

	// Something went wrong (connection error or reachability error)
	// If socket connection failed completely, cannot proceed
	if sess == nil {
		return nil, false, err
	}

	// Socket connected but device unreachable
	// Should we test unconnected mode?
	if m.shouldTestUnconnected(err) {
		// First time encountering timeout - test unconnected once
		return m.testUnconnectedFallback(sess)
	}

	// Already tested or shouldn't test - return unreachable
	m.session = sess
	return sess, false, nil
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

// connectWithUnconnectedSocket connects using unconnected UDP socket mode.
// Called on subsequent Connect() calls after we've decided to use unconnected mode permanently.
func (m *snmpConnectionManager) connectWithUnconnectedSocket() (session.Session, bool, error) {
	sess, reachable, err := m.tryConnect(m.config) // m.config has UseUnconnectedUDPSocket = true
	if sess == nil {
		return nil, false, err
	}
	m.session = sess
	return sess, reachable, nil
}

// shouldTestUnconnected determines if we should test unconnected socket mode.
// Returns true only on the first timeout, before we've made a decision.
func (m *snmpConnectionManager) shouldTestUnconnected(reachErr error) bool {
	// Already tested unconnected mode
	if m.fallbackState.fallbackTestAttempted {
		return false
	}
	// Connected socket worked in a previous call - no need to test
	if m.fallbackState.connectedSocketSucceeded {
		return false
	}
	// Only test on timeout errors (not auth errors, SNMP errors, etc.)
	return isTimeoutError(reachErr)
}

// testUnconnectedFallback tests unconnected socket mode once and permanently decides which mode to use.
// Called only on the first timeout. Future calls will use the decided mode.
func (m *snmpConnectionManager) testUnconnectedFallback(connectedSess session.Session) (session.Session, bool, error) {
	log.Infof("[%s] Connected socket timed out, testing unconnected socket", m.config.IPAddress)
	m.fallbackState.fallbackTestAttempted = true

	// Create temporary config for testing
	testConfig := m.config.Copy()
	testConfig.UseUnconnectedUDPSocket = true

	unconnectedSess, reachable, err := m.tryConnect(testConfig)
	if unconnectedSess == nil {
		// Unconnected socket failed to connect - keep using connected mode
		log.Infof("[%s] Unconnected socket failed: %s, keeping connected mode", m.config.IPAddress, err)
		m.session = connectedSess
		return connectedSess, false, nil
	}

	if !reachable {
		// Unconnected socket also unreachable - keep using connected mode
		log.Infof("[%s] Unconnected socket also unreachable, keeping connected mode", m.config.IPAddress)
		unconnectedSess.Close()
		m.session = connectedSess
		return connectedSess, false, nil
	}

	// Device is reachable with unconnected socket - switch permanently
	log.Infof("[%s] Unconnected socket succeeded, switching to unconnected mode permanently", m.config.IPAddress)
	m.config.UseUnconnectedUDPSocket = true
	m.fallbackState.useUnconnectedSocket = true

	connectedSess.Close()
	m.session = unconnectedSess
	return unconnectedSess, true, nil
}

// tryConnect attempts to create a session, connect, and check reachability.
// Returns (session, reachable, error).
// - error == nil && reachable: Success - socket connected and device reachable
// - error != nil && session == nil: Socket connection failed
// - error != nil && session != nil: Socket connected but device unreachable
func (m *snmpConnectionManager) tryConnect(config *checkconfig.CheckConfig) (session.Session, bool, error) {
	sess, err := m.sessionFactory(config)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create session: %w", err)
	}

	connectErr := sess.Connect()
	if connectErr != nil {
		return nil, false, connectErr
	}

	// Connection succeeded - check reachability
	_, reachErr := sess.GetNext([]string{coresnmp.DeviceReachableGetNextOid})
	return sess, reachErr == nil, reachErr
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
