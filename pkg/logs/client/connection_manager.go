// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package client

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"golang.org/x/net/proxy"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	backoffUnit       = 2 * time.Second
	backoffMax        = 30 * time.Second
	connectionTimeout = 20 * time.Second
)

// A ConnectionManager manages connections
type ConnectionManager struct {
	endpoint  Endpoint
	mutex     sync.Mutex
	firstConn sync.Once
}

// NewConnectionManager returns an initialized ConnectionManager
func NewConnectionManager(endpoint Endpoint) *ConnectionManager {
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

	var retries int
	for {
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
		var err error

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
	log.Info("Connection closed")
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

// backoff lets the connection manager sleep a bit
func (cm *ConnectionManager) backoff(ctx context.Context, retries int) {
	backoffDuration := backoffUnit * time.Duration(retries)
	if backoffDuration > backoffMax {
		backoffDuration = backoffMax
	}
	time.Sleep(backoffDuration)

	ctx, cancel := context.WithTimeout(ctx, backoffDuration)
	defer cancel()
	<-ctx.Done()
}
