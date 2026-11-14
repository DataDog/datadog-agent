// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tcp provides TCP connection management for log clients
package tcp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"

	"golang.org/x/net/proxy"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	maxExpBackoffCount    = 7
	connectionTimeout     = 20 * time.Second
	statusConnectionError = "connection_error"
)

// A ConnectionManager manages connections
type ConnectionManager struct {
	endpoint  config.Endpoint
	mutex     sync.Mutex
	firstConn sync.Once
	status    statusinterface.Status
}

// NewConnectionManager returns an initialized ConnectionManager
func NewConnectionManager(endpoint config.Endpoint, status statusinterface.Status) *ConnectionManager {
	return &ConnectionManager{
		endpoint: endpoint,
		status:   status,
	}
}

type tlsTimeoutError struct{}

func (tlsTimeoutError) Error() string {
	return "tls: Handshake timed out"
}

// NewConnection returns an initialized connection to the intake.
// It blocks until a connection is available
func (cm *ConnectionManager) NewConnection(ctx context.Context) (net.Conn, error) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	cm.firstConn.Do(func() {
		if cm.endpoint.ProxyAddress != "" {
			log.Infof("Connecting to the backend: %v, via socks5: %v, with SSL: %v", cm.address(), cm.endpoint.ProxyAddress, cm.endpoint.UseSSL())
		} else {
			log.Infof("Connecting to the backend: %v, with SSL: %v", cm.address(), cm.endpoint.UseSSL())
		}
	})

	var retries uint
	var err error
	for {
		if err != nil {
			cm.status.AddGlobalWarning(statusConnectionError, fmt.Sprintf("Connection to the log intake cannot be established: %v", err))
		}
		if retries > 0 {
			log.Debugf("Connect attempt #%d", retries)
			cm.backoff(ctx, retries)
		}
		retries++

		// Check if we should continue.
		select {
		// This is the normal shutdown path when the caller is stopped.
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			// Continue.
		}

		var conn net.Conn
		conn, err = cm.dial(ctx)
		if err != nil {
			log.Warn(err)
			continue
		}
		log.Debugf("connected to %v", cm.address())

		if cm.endpoint.UseSSL() {
			sslConn := tls.Client(conn, &tls.Config{
				ServerName: cm.endpoint.Host,
			})
			err = cm.handshakeWithTimeout(ctx, sslConn, connectionTimeout)
			if err != nil {
				sslConn.Close()
				log.Warn(err)
				continue
			}
			log.Debug("SSL handshake successful")
			conn = sslConn
		}

		stopClose := context.AfterFunc(ctx, func() { cm.CloseConnection(conn) })
		go func() {
			defer stopClose()
			cm.handleServerClose(conn)
		}()
		cm.status.RemoveGlobalWarning(statusConnectionError)
		return conn, nil
	}
}

func (cm *ConnectionManager) dial(ctx context.Context) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, connectionTimeout)
	defer cancel()
	direct := &net.Dialer{}
	if cm.endpoint.ProxyAddress == "" {
		return direct.DialContext(ctx, "tcp", cm.address())
	}
	dialer, err := proxy.SOCKS5("tcp", cm.endpoint.ProxyAddress, nil, direct)
	if err != nil {
		return nil, err
	}
	contextDialer, ok := dialer.(proxy.ContextDialer)
	if !ok {
		return nil, errors.New("SOCKS5 dialer does not support cancellation")
	}
	return contextDialer.DialContext(ctx, "tcp", cm.address())
}

func (cm *ConnectionManager) handshakeWithTimeout(ctx context.Context, conn *tls.Conn, timeout time.Duration) error {
	handshakeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	log.Debug("Start TLS handshake")
	err := conn.HandshakeContext(handshakeCtx)
	log.Debug("TLS handshake ended")
	if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
		return tlsTimeoutError{}
	}
	return err
}

// address returns the address of the server to send logs to.
func (cm *ConnectionManager) address() string {
	return net.JoinHostPort(cm.endpoint.Host, strconv.Itoa(cm.endpoint.Port))
}

// ShouldReset returns whether the connection should be reset, depending on the endpoint's config
// and the passed connection creation time.
func (cm *ConnectionManager) ShouldReset(connCreationTime time.Time) bool {
	return cm.endpoint.ConnectionResetInterval != 0 && time.Since(connCreationTime) > cm.endpoint.ConnectionResetInterval
}

// CloseConnection closes a connection on the client side
func (cm *ConnectionManager) CloseConnection(conn net.Conn) {
	conn.Close()
	log.Debug("Connection closed")
}

// handleServerClose closes the connection on peer shutdown or an unexpected read error.
func (cm *ConnectionManager) handleServerClose(conn net.Conn) {
	for {
		buff := make([]byte, 1)
		_, err := conn.Read(buff)
		switch {
		case err == nil:
		case errors.Is(err, net.ErrClosed):
			// Connection already closed, expected
			return
		case err == io.EOF:
			cm.CloseConnection(conn)
			return
		default:
			log.Warn(err)
			cm.CloseConnection(conn)
			return
		}
	}
}

// backoff implements a randomized exponential backoff in case of connection failure
// each invocation will trigger a sleep between [2^(retries-1), 2^retries) second
// the exponent is capped at 7, which translates to max sleep between ~1min and ~2min
func (cm *ConnectionManager) backoff(ctx context.Context, retries uint) {
	if retries > maxExpBackoffCount {
		retries = maxExpBackoffCount
	}

	backoffMax := 1 << retries
	backoffMin := 1 << (retries - 1)
	backoffDuration := time.Duration(backoffMin+rand.Intn(backoffMax-backoffMin)) * time.Second

	ctx, cancel := context.WithTimeout(ctx, backoffDuration)
	defer cancel()
	<-ctx.Done()
}
