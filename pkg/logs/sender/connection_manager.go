// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package sender

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/net/proxy"
)

const (
	backoffUnit = 2 * time.Second
	backoffMax  = 30 * time.Second
	timeout     = 20 * time.Second
)

// A ConnectionManager manages connections
type ConnectionManager struct {
	host         string
	port         int
	useSSL       bool
	proxyAddress string
	mutex        sync.Mutex
	firstConn    sync.Once
}

// NewConnectionManager returns an initialized ConnectionManager
func NewConnectionManager(host string, port int, useSSL bool, proxyAddress string) *ConnectionManager {
	return &ConnectionManager{
		host:         host,
		port:         port,
		useSSL:       useSSL,
		proxyAddress: proxyAddress,
	}
}

// NewConnection returns an initialized connection to the intake.
// It blocks until a connection is available
func (cm *ConnectionManager) NewConnection() net.Conn {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	cm.firstConn.Do(func() {
		if cm.proxyAddress != "" {
			log.Infof("Connecting to the backend: %v, via socks5: %v, with SSL: %v", cm.address(), cm.proxyAddress, cm.useSSL)
		} else {
			log.Infof("Connecting to the backend: %v, with SSL: %v", cm.address(), cm.useSSL)
		}
	})

	var retries int
	for {
		if retries > 0 {
			cm.backoff(retries)
		}
		retries++

		var conn net.Conn
		var err error

		if cm.proxyAddress != "" {
			var dialer proxy.Dialer
			dialer, err = proxy.SOCKS5("tcp", cm.proxyAddress, nil, proxy.Direct)
			if err != nil {
				log.Warn(err)
				continue
			}
			conn, err = dialer.Dial("tcp", cm.address())
		} else {
			conn, err = net.DialTimeout("tcp", cm.address(), timeout)
		}
		if err != nil {
			log.Warn(err)
			continue
		}

		if cm.useSSL {
			sslConn := tls.Client(conn, &tls.Config{
				ServerName: cm.host,
			})
			err = sslConn.Handshake()
			if err != nil {
				log.Warn(err)
				continue
			}
			conn = sslConn
		}

		go cm.handleServerClose(conn)
		return conn
	}
}

// address returns the address of the server to send logs to.
func (cm *ConnectionManager) address() string {
	return fmt.Sprintf("%s:%d", cm.host, cm.port)
}

// CloseConnection closes a connection on the client side
func (cm *ConnectionManager) CloseConnection(conn net.Conn) {
	conn.Close()
}

// handleServerClose lets the connection manager detect when a connection
// has been closed by the server, and closes it for the client.
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
func (cm *ConnectionManager) backoff(retries int) {
	backoffDuration := backoffUnit * time.Duration(retries)
	if backoffDuration > backoffMax {
		backoffDuration = backoffMax
	}
	time.Sleep(backoffDuration)
}
