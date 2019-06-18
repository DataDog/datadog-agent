// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package tcp

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"

	"golang.org/x/net/proxy"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
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
}

// NewConnectionManager returns an initialized ConnectionManager
func NewConnectionManager(endpoint config.Endpoint) *ConnectionManager {
	return &ConnectionManager{
		endpoint: endpoint,
	}
}

// NewConnection returns an initialized connection to the intake.
// It blocks until a connection is available
func (cm *ConnectionManager) NewConnection(ctx context.Context) (net.Conn, error) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	cm.firstConn.Do(func() {
		if cm.endpoint.ProxyAddress != "" {
			log.Infof("Connecting to the backend: %v, via socks5: %v, with SSL: %v", cm.address(), cm.endpoint.ProxyAddress, cm.endpoint.UseSSL)
		} else {
			log.Infof("Connecting to the backend: %v, with SSL: %v", cm.address(), cm.endpoint.UseSSL)
		}
	})

	var retries uint
	var err error
	for {
		if err != nil {
			status.AddGlobalWarning(statusConnectionError, fmt.Sprintf("Connection to the log intake cannot be established: %v", err))
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

		if cm.endpoint.ProxyAddress != "" {
			var dialer proxy.Dialer
			dialer, err = proxy.SOCKS5("tcp", cm.endpoint.ProxyAddress, nil, proxy.Direct)
			if err != nil {
				log.Warn(err)
				continue
			}
			// TODO: handle timeouts with ctx.
			conn, err = dialer.Dial("tcp", cm.address())
		} else {
			var dialer net.Dialer
			dctx, cancel := context.WithTimeout(ctx, connectionTimeout)
			defer cancel()
			conn, err = dialer.DialContext(dctx, "tcp", cm.address())
		}
		if err != nil {
			log.Warn(err)
			continue
		}
		log.Debug("connected to %v", cm.address())

		if cm.endpoint.UseSSL {
			sslConn := tls.Client(conn, &tls.Config{
				ServerName: cm.endpoint.Host,
			})
			// TODO: handle timeouts with ctx.
			err = sslConn.Handshake()
			if err != nil {
				log.Warn(err)
				continue
			}
			log.Debug("SSL handshake successful")
			conn = sslConn
		}

		go cm.handleServerClose(conn)
		status.RemoveGlobalWarning(statusConnectionError)
		return conn, nil
	}
}

// address returns the address of the server to send logs to.
func (cm *ConnectionManager) address() string {
	return net.JoinHostPort(cm.endpoint.Host, strconv.Itoa(cm.endpoint.Port))
}

// CloseConnection closes a connection on the client side
func (cm *ConnectionManager) CloseConnection(conn net.Conn) {
	conn.Close()
	log.Debug("Connection closed")
}

// handleServerClose lets the connection manager detect when a connection
// has been closed by the server, and closes it for the client.
// This is not strictly necessary but a good safeguard against callers
// that might not handle errors properly.
func (cm *ConnectionManager) handleServerClose(conn net.Conn) {
	for {
		buff := make([]byte, 1)
		_, err := conn.Read(buff)
		if err == io.EOF {
			cm.CloseConnection(conn)
			return
		} else if err != nil {
			log.Warn(err)
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
