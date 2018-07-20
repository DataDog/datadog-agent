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

	"golang.org/x/net/proxy"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	backoffUnit = 2 * time.Second
	backoffMax  = 30 * time.Second
	timeout     = 20 * time.Second
)

// A ConnectionManager manages connections
type ConnectionManager struct {
	serverName    string
	serverAddress string
	devModeNoSSL  bool
	proxyAddress  string
	mutex         sync.Mutex
	firstConn     sync.Once
}

// NewConnectionManager returns an initialized ConnectionManager
func NewConnectionManager(serverName string, serverPort int, devModeNoSSL bool, proxyAddress string) *ConnectionManager {
	return &ConnectionManager{
		serverName:    serverName,
		serverAddress: fmt.Sprintf("%s:%d", serverName, serverPort),
		proxyAddress:  proxyAddress,
		devModeNoSSL:  devModeNoSSL,
	}
}

// NewConnection returns an initialized connection to the intake.
// It blocks until a connection is available
func (cm *ConnectionManager) NewConnection() net.Conn {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	cm.firstConn.Do(func() {
		if cm.proxyAddress != "" {
			log.Infof("Connecting to the backend: %v, via socks5: %v", cm.serverAddress, cm.proxyAddress)
		} else {
			log.Infof("Connecting to the backend: %v", cm.serverAddress)
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
			conn, err = dialer.Dial("tcp", cm.serverAddress)
		} else {
			conn, err = net.DialTimeout("tcp", cm.serverAddress, timeout)
		}
		if err != nil {
			log.Warn(err)
			continue
		}

		if !cm.devModeNoSSL {
			sslConn := tls.Client(conn, &tls.Config{
				ServerName: cm.serverName,
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
