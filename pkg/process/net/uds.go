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
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// UDSListener (Unix Domain Socket Listener)
type UDSListener struct {
	conn       net.Listener
	socketPath string
}

func IsUnixNetConnValid(unixConn *net.UnixConn, allowedUsrID int, allowedGrpID int) (bool, error) {
	sysConn, err := unixConn.SyscallConn()
	if err != nil {
		return false, err
	}
	var ucred *syscall.Ucred
	var ucredErr error
	err = sysConn.Control(func(fd uintptr) {
		ucred, ucredErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	})
	if err != nil || ucredErr != nil {
		return false, err
	}
	if (ucred.Uid == 0 && ucred.Gid == 0) ||
		(ucred.Uid == uint32(allowedUsrID) && ucred.Gid == uint32(allowedGrpID)) {

		return false, nil
	}
	return true, nil
}

// HttpServe is equivalent to http.Serve()
// but will check credential (root:root or allowedUsrID:allowedGrpID) are allowed to access the unix socket
func HttpServe(l net.Listener, handler http.Handler, allowedUsrID int, allowedGrpID int) error {
	srv := &http.Server{
		Handler: handler,
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			var unixConn *net.UnixConn
			var ok bool
			if unixConn, ok = c.(*net.UnixConn); !ok {
				return ctx
			}
			valid, err := IsUnixNetConnValid(unixConn, allowedUsrID, allowedGrpID)
			if err != nil || !valid {
				if err != nil {
					log.Errorf("unix socket %s -> %s closing connection, error %s", unixConn.LocalAddr(), unixConn.RemoteAddr(), err)
				}
				if !valid {
					log.Debugf("unix socket %s -> %s closing connection, rejected", unixConn.LocalAddr(), unixConn.RemoteAddr())
				}
				// reject the connection
				c.Close()
			}
			return ctx
		},
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
