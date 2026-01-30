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

// udpSocketFallbackConnector interface for dependency injection and testing
type udpSocketFallbackConnector interface {
	// connectWithFallback attempts connection with automatic unconnected UDP fallback.
	// Returns the connected session (potentially recreated), whether unconnected UDP
	// fallback was enabled during this call, and any connection error.
	connectWithFallback(sess session.Session) (session.Session, bool, error)
}

// udpSocketFallbackState tracks whether we should use unconnected UDP sockets.
// Unconnected sockets accept responses from any source IP, working around devices
// that respond from IP B when queried on IP A (multi-homed devices). This prevents
// connection timeouts for devices with multiple management interfaces.
type udpSocketFallbackState struct {
	connectedSocketSucceeded bool // True if standard connected socket ever worked
	fallbackTestAttempted    bool // True if we already tested unconnected fallback
	useUnconnectedSocket     bool // True if we should use unconnected UDP mode
}

// udpSocketFallbackManager handles automatic fallback to unconnected UDP sockets
// for multi-homed network devices that respond from different IPs than requested
type udpSocketFallbackManager struct {
	state          udpSocketFallbackState
	config         *checkconfig.CheckConfig
	sessionFactory session.Factory
}

// newUDPSocketFallbackManager creates a new fallback manager
func newUDPSocketFallbackManager(config *checkconfig.CheckConfig, factory session.Factory) udpSocketFallbackConnector {
	return &udpSocketFallbackManager{
		state:          udpSocketFallbackState{},
		config:         config,
		sessionFactory: factory,
	}
}

// connectWithFallback attempts to connect session and verify reachability, applying fallback logic if needed.
// Returns the connected session (potentially recreated), whether unconnected UDP
// fallback was enabled during this call, and any connection error.
func (m *udpSocketFallbackManager) connectWithFallback(sess session.Session) (session.Session, bool, error) {
	// Try to connect
	err := sess.Connect()
	if err == nil {
		// Connection succeeded - now test reachability for UDP
		// This is critical because UDP Connect() doesn't actually establish a connection
		// The timeout happens during the first read operation (Get/GetNext)
		_, reachErr := sess.GetNext([]string{coresnmp.DeviceReachableGetNextOid})
		if reachErr == nil {
			// Both connect and reachability check succeeded
			if !m.state.useUnconnectedSocket {
				m.state.connectedSocketSucceeded = true
			}
			return sess, false, nil
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
		return sess, false, err
	}

	// Attempt fallback
	log.Infof("[%s] Connected socket failed with timeout, testing unconnected socket fallback", m.config.IPAddress)

	if !m.testFallback() {
		log.Infof("[%s] Unconnected socket test failed, keeping connected mode", m.config.IPAddress)
		return sess, false, err
	}

	// Fallback test succeeded - switch to unconnected mode
	log.Infof("[%s] Unconnected socket test succeeded, switching to unconnected mode", m.config.IPAddress)
	m.state.useUnconnectedSocket = true

	// Update config and recreate session
	m.config.UseUnconnectedUDPSocket = true
	newSess, createErr := m.sessionFactory(m.config)
	if createErr != nil {
		return sess, false, fmt.Errorf("failed to recreate session with unconnected socket for %s: %w", m.config.IPAddress, createErr)
	}

	// Try connecting with new session
	if connectErr := newSess.Connect(); connectErr != nil {
		return newSess, false, connectErr
	}

	return newSess, true, nil
}

// shouldAttemptFallback determines if fallback should be attempted
func (m *udpSocketFallbackManager) shouldAttemptFallback(err error) bool {
	if m.state.useUnconnectedSocket {
		return false // Already using unconnected
	}
	if m.state.fallbackTestAttempted {
		return false // Already tested
	}
	if m.state.connectedSocketSucceeded {
		return false // Connected socket worked before
	}
	if !isTimeoutError(err) {
		return false // Not a timeout error
	}
	return true
}

// testFallback tests if unconnected socket works
func (m *udpSocketFallbackManager) testFallback() bool {
	m.state.fallbackTestAttempted = true

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
