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
func (m *snmpConnectionManager) Connect() (session.Session, bool, error) {
	// Try connected socket first (unless already using unconnected)
	sess, reachable, reachErr := m.tryConnect(m.config)
	if sess == nil {
		// Socket connection failed - cannot connect at all
		return nil, false, reachErr
	}

	if reachable {
		// Connected socket works perfectly
		if !m.fallbackState.useUnconnectedSocket {
			m.fallbackState.connectedSocketSucceeded = true
		}
		m.session = sess
		return sess, true, nil
	}

	// Reachability check failed - check if we should attempt fallback
	if !m.shouldAttemptFallback(reachErr) {
		// Not a timeout or shouldn't fallback - return unreachable
		m.session = sess
		return sess, false, nil
	}

	// Timeout occurred - try unconnected socket once
	log.Infof("[%s] Connected socket failed with timeout, trying unconnected socket", m.config.IPAddress)
	m.fallbackState.fallbackTestAttempted = true

	// Create temporary config for testing unconnected socket
	testConfig := m.config.Copy()
	testConfig.UseUnconnectedUDPSocket = true

	unconnectedSess, unconnectedReachable, unconnectedErr := m.tryConnect(testConfig)
	if unconnectedSess == nil {
		// Unconnected socket failed to connect - fall back to connected
		log.Infof("[%s] Unconnected socket failed: %s, keeping connected mode", m.config.IPAddress, unconnectedErr)
		m.session = sess
		return sess, false, nil
	}

	// Check if device is reachable with unconnected socket
	if !unconnectedReachable {
		// Unconnected socket also unreachable - fall back to connected
		log.Infof("[%s] Unconnected socket also unreachable, keeping connected mode", m.config.IPAddress)
		unconnectedSess.Close()
		m.session = sess
		return sess, false, nil
	}

	// Device is reachable with unconnected socket - switch to it permanently
	log.Infof("[%s] Unconnected socket succeeded, switching to unconnected mode", m.config.IPAddress)
	m.config.UseUnconnectedUDPSocket = true
	m.fallbackState.useUnconnectedSocket = true

	// Close the old connected session and use unconnected
	if err := sess.Close(); err != nil {
		log.Debugf("[%s] Failed to close connected session: %s", m.config.IPAddress, err)
	}
	m.session = unconnectedSess
	return unconnectedSess, true, nil
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

// tryConnect attempts to create a session, connect, and check reachability.
// Returns (session, reachable, error).
// If session is nil, connection failed and error is set.
// If session is non-nil, connection succeeded; reachable indicates if device responded.
func (m *snmpConnectionManager) tryConnect(config *checkconfig.CheckConfig) (session.Session, bool, error) {
	sess, err := m.sessionFactory(config)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create session: %w", err)
	}

	connectErr := sess.Connect()
	if connectErr != nil {
		return nil, false, connectErr
	}

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
