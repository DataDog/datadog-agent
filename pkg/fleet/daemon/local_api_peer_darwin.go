// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package daemon

import (
	"context"
	"net"

	"golang.org/x/sys/unix"
)

// rootOnlyChanges is set on macOS, where the daemon runs as root while its socket belongs to the
// Agent's account, which only ever reads the daemon's status: every other route is reserved for
// root callers.
const rootOnlyChanges = true

// connContext records the uid of the process at the other end of each connection in the context
// of every request made over it.
func connContext(ctx context.Context, conn net.Conn) context.Context {
	if uid, ok := peerUID(conn); ok {
		return context.WithValue(ctx, peerUIDKey{}, uid)
	}
	return ctx
}

// peerUID returns the effective uid of the process at the other end of a Unix socket connection.
func peerUID(conn net.Conn) (uint32, bool) {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return 0, false
	}
	raw, err := unixConn.SyscallConn()
	if err != nil {
		return 0, false
	}
	var cred *unix.Xucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, credErr = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
	}); err != nil || credErr != nil {
		return 0, false
	}
	return cred.Uid, true
}
