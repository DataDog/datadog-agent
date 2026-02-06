// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package devicecheck

import (
	"errors"
	"fmt"
	"strings"

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

	// MarkConnectionBroken marks the session as broken and needing reconnection.
	MarkConnectionBroken()

	// Close closes the current session.
	Close() error
}

// connectionMode represents the decided connection mode after the first run.
type connectionMode int

const (
	// modeUndecided means we haven't attempted connection yet (first run pending).
	modeUndecided connectionMode = iota
	// modeConnected means connected sockets work and should always be used.
	modeConnected
	// modeUnconnected means connected sockets timed out but unconnected worked.
	modeUnconnected
)

// snmpConnectionManager implements ConnectionManager with automatic fallback
// to unconnected UDP sockets for unexpected network behavior.
//
// Some network configurations cause devices to respond from a different IP
// than the one queried. Connected UDP sockets fail in this case because they
// expect responses from the same IP. Unconnected sockets accept responses
// from any IP and work as a fallback.
//
// On first Connect():
//   - Tries connected socket first (normal case)
//   - If timeout occurs, tests unconnected socket once
//   - Permanently decides which mode to use based on test result
//
// On subsequent Connect() calls:
//   - Uses the previously decided mode (no more testing)
type snmpConnectionManager struct {
	config         *checkconfig.CheckConfig
	sessionFactory session.Factory
	session        session.Session
	mode           connectionMode
	isConnected    bool // Track if session is currently connected
}

// NewConnectionManager creates a new SNMP connection manager.
func NewConnectionManager(config *checkconfig.CheckConfig, sessionFactory session.Factory) ConnectionManager {
	return &snmpConnectionManager{
		config:         config.Copy(),
		sessionFactory: sessionFactory,
		mode:           modeUndecided,
	}
}

// Connect establishes a connection and returns (session, deviceReachable, error).
func (m *snmpConnectionManager) Connect() (session.Session, bool, error) {
	// If already connected, return existing session without checking reachability
	// (reachability will be detected when actual SNMP calls are made)
	if m.isConnected && m.session != nil {
		return m.session, true, nil
	}

	// Not connected - establish new connection
	if m.mode == modeUndecided {
		return m.firstConnect()
	}

	useUnconnected := m.mode == modeUnconnected
	sess, err := m.createSession(useUnconnected)
	if err != nil {
		m.isConnected = false
		return nil, false, err
	}
	m.session = sess

	reachErr := m.checkReachability(sess)
	m.isConnected = true // Mark connected even if unreachable

	return sess, reachErr == nil, nil
}

// GetSession returns the current active session.
func (m *snmpConnectionManager) GetSession() (session.Session, error) {
	if m.session == nil {
		return nil, errors.New("session not initialized - call Connect() first")
	}
	return m.session, nil
}

// Close closes the current session.
func (m *snmpConnectionManager) Close() error {
	if m.session == nil {
		return nil
	}
	err := m.session.Close()
	m.session = nil
	m.isConnected = false
	return err
}

// firstConnect handles the initial connection attempt and mode decision.
// This is the only place where fallback testing occurs.
func (m *snmpConnectionManager) firstConnect() (session.Session, bool, error) {
	// Always try connected socket first
	connectedSession, err := m.createSession(false)
	if err != nil {
		log.Errorf("Failed to create connected session: %s", err)
		// Socket creation failed entirely - no point testing fallback
		// Because this is UDP, this is not influenced by the state of the remote device
		return nil, false, err
	}

	reachErr := m.checkReachability(connectedSession)
	if reachErr == nil {
		// Connected socket works perfectly - use it permanently
		m.mode = modeConnected
		m.session = connectedSession
		m.isConnected = true
		return connectedSession, true, nil
	}

	// Device unreachable - should we test unconnected mode?
	if !isTimeoutError(reachErr) {
		// Not a timeout (e.g., auth error) - fallback won't help
		m.mode = modeConnected
		m.session = connectedSession
		m.isConnected = true
		return connectedSession, false, nil
	}

	// Network timeout occurred - test unconnected socket
	log.Infof("[%s] Connected socket timed out, testing unconnected socket", m.config.IPAddress)

	unconnSess, unconnErr := m.createSession(true)
	if unconnErr != nil {
		// Unconnected socket failed to create - stick with connected
		log.Infof("[%s] Unconnected socket creation failed, defaulting to connected mode", m.config.IPAddress)
		m.mode = modeConnected
		m.session = connectedSession
		m.isConnected = true
		return connectedSession, false, nil
	}

	unconnReachErr := m.checkReachability(unconnSess)
	if unconnReachErr == nil {
		// Unconnected works - switch permanently
		log.Infof("[%s] Unconnected socket succeeded, switching permanently", m.config.IPAddress)
		m.mode = modeUnconnected
		connectedSession.Close()
		m.session = unconnSess
		m.isConnected = true
		return unconnSess, true, nil
	}

	// Unconnected also failed - default to connected mode
	log.Infof("[%s] Unconnected socket also failed, defaulting to connected mode", m.config.IPAddress)
	m.mode = modeConnected
	unconnSess.Close()
	m.session = connectedSession
	m.isConnected = true
	return connectedSession, false, nil
}

// createSession creates and connects an SNMP session.
// Returns (session, nil) on success or (nil, error) on failure - never both.
func (m *snmpConnectionManager) createSession(useUnconnected bool) (session.Session, error) {
	cfg := m.config.Copy()
	cfg.UseUnconnectedUDPSocket = useUnconnected

	sess, err := m.sessionFactory(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	if err := sess.Connect(); err != nil {
		return nil, err
	}

	return sess, nil
}

// checkReachability verifies the device responds to SNMP queries.
// Returns nil if reachable, or the error if not.
func (m *snmpConnectionManager) checkReachability(sess session.Session) error {
	_, err := sess.GetNext([]string{coresnmp.DeviceReachableGetNextOid})
	return err
}

// isTimeoutError checks if the error is a network timeout.
func isTimeoutError(err error) bool {
	// gosnmp doesn't implement the net.Error interface, so we check the error string
	// https://github.com/gosnmp/gosnmp/blob/e72026a86bb80209ed38f118892479e6b7177344/marshal.go#L210
	return err != nil && strings.Contains(err.Error(), "timeout")
}

// MarkConnectionBroken marks the session as broken and needing reconnection.
func (m *snmpConnectionManager) MarkConnectionBroken() {
	m.isConnected = false
}

// isConnectionError checks if error indicates broken connection.
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	connectionErrors := []string{
		"connection refused", "connection reset", "broken pipe",
		"socket closed", "i/o timeout", "connection timed out",
		"no route to host", "network is unreachable",
	}
	for _, connErr := range connectionErrors {
		if strings.Contains(errStr, connErr) {
			return true
		}
	}
	return false
}
