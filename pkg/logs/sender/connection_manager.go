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

	log "github.com/cihub/seelog"
)

const (
	backoffSleepTimeUnit = 2  // in seconds
	maxBackoffSleepTime  = 30 // in seconds
	timeout              = 20 * time.Second
)

// A ConnectionManager manages connections
type ConnectionManager struct {
	connectionString string
	serverName       string
	devModeNoSSL     bool

	mutex   sync.Mutex
	retries int

	firstConn bool
}

// NewConnectionManager returns an initialized ConnectionManager
func NewConnectionManager(ddURL string, ddPort int, devModeNoSSL bool) *ConnectionManager {
	return &ConnectionManager{
		connectionString: fmt.Sprintf("%s:%d", ddURL, ddPort),
		serverName:       ddURL,
		devModeNoSSL:     devModeNoSSL,

		mutex: sync.Mutex{},

		firstConn: true,
	}
}

// NewConnection returns an initialized connection to the intake.
// It blocks until a connection is available
func (cm *ConnectionManager) NewConnection() net.Conn {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	for {
		if cm.firstConn {
			log.Info("Connecting to the backend: ", cm.connectionString)
			cm.firstConn = false
		}

		cm.retries++
		outConn, err := net.DialTimeout("tcp", cm.connectionString, timeout)
		if err != nil {
			log.Warn(err)
			cm.backoff()
			continue
		}

		if !cm.devModeNoSSL {
			config := &tls.Config{
				ServerName: cm.serverName,
			}
			sslConn := tls.Client(outConn, config)
			err = sslConn.Handshake()
			if err != nil {
				log.Warn(err)
				cm.backoff()
				continue
			}
			outConn = sslConn
		}

		cm.retries = 0
		go cm.handleServerClose(outConn)
		return outConn
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

// backoff lets the connection mananger sleep a bit
func (cm *ConnectionManager) backoff() {
	backoffDuration := backoffSleepTimeUnit * cm.retries
	if backoffDuration > maxBackoffSleepTime {
		backoffDuration = maxBackoffSleepTime
	}
	timer := time.NewTimer(time.Second * time.Duration(backoffDuration))
	<-timer.C
}
