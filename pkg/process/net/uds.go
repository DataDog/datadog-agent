// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || darwin

package net

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const udsAgentSig = "UDS_AGENT_SIG-e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca4"
const udsProcessAgentSig = "UDS_PROCESS_AGENT_SIG-6df08279acf372b0fe1c624369059fe2d6ade65d05"

// UDSListener (Unix Domain Socket Listener)
type UDSListener struct {
	conn       net.Listener
	socketPath string
}

// HttpServe is equivalent to http.Serve()
// but will check credential if authSocket is true by verifying the client pid binary signature
func HttpServe(l net.Listener, handler http.Handler, authSocket bool) error {
	srv := &http.Server{Handler: handler}
	if authSocket {
		srv.ConnContext = func(ctx context.Context, c net.Conn) context.Context {
			var unixConn *net.UnixConn
			var ok bool
			if unixConn, ok = c.(*net.UnixConn); !ok {
				return ctx
			}
			log.Debugf("unix socket %s -> %s connection : signature %s", unixConn.LocalAddr(), unixConn.RemoteAddr(), udsProcessAgentSig)
			valid, err := IsUnixNetConnValid(unixConn, udsAgentSig, udsProcessAgentSig)
			if err != nil || !valid {
				if err != nil {
					log.Errorf("unix socket %s -> %s closing connection, error %s", unixConn.LocalAddr(), unixConn.RemoteAddr(), err)
				} else if !valid {
					log.Errorf("unix socket %s -> %s closing connection, rejected. Client accessing this socket require a signed binary", unixConn.LocalAddr(), unixConn.RemoteAddr())
				}
				// reject the connection
				newCtx, cancelCtx := context.WithCancel(ctx)
				ctx = newCtx
				cancelCtx()
				c.Close()
			}
			if valid {
				log.Debugf("unix socket %s -> %s connection authenticated", unixConn.LocalAddr(), unixConn.RemoteAddr())
			}
			return ctx
		}
	}
	return srv.Serve(l)
}

// NewListener returns an idle UDSListener
func NewListener(socketAddr string) (*UDSListener, error) {
	if len(socketAddr) == 0 {
		return nil, fmt.Errorf("uds: empty socket path provided")
	}

	addr, err := net.ResolveUnixAddr("unix", socketAddr)
	if err != nil {
		return nil, fmt.Errorf("uds: can't ResolveUnixAddr: %v", err)
	}

	// Check to see if there's a pre-existing system probe socket.
	fileInfo, err := os.Stat(socketAddr)
	if err == nil { // No error means the socket file already exists
		// If it's not a UNIX socket, then this is a problem.
		if fileInfo.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("uds: cannot reuse %s socket path: path already exists and it is not a UNIX socket", socketAddr)
		}
		// Attempt to remove the pre-existing socket
		if err = os.Remove(socketAddr); err != nil {
			return nil, fmt.Errorf("uds: cannot remove stale UNIX socket: %v", err)
		}
	}

	conn, err := net.Listen("unix", addr.Name)
	if err != nil {
		return nil, fmt.Errorf("can't listen: %s", err)
	}

	if err := os.Chmod(socketAddr, 0720); err != nil {
		return nil, fmt.Errorf("can't set the socket at write only: %s", err)
	}

	perms, err := filesystem.NewPermission()
	if err != nil {
		return nil, err
	}

	if err := perms.RestrictAccessToUser(socketAddr); err != nil {
		return nil, err
	}

	listener := &UDSListener{
		conn:       conn,
		socketPath: socketAddr,
	}

	log.Debugf("uds: %s successfully initialized", conn.Addr())
	return listener, nil
}

// GetListener will return the underlying Conn's net.Listener
func (l *UDSListener) GetListener() net.Listener {
	return l.conn
}

// Stop closes the UDSListener connection and stops listening
func (l *UDSListener) Stop() {
	_ = l.conn.Close()

	// Socket cleanup on exit - above conn.Close() should remove it, but just in case.
	if err := os.Remove(l.socketPath); err != nil {
		log.Debugf("uds: error removing socket file: %s", err)
	}
}
