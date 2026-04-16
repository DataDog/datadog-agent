// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package impl

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// verifyCaller uses SO_PEERCRED to confirm the connecting process runs as the
// same UID as par-executor. This prevents any other user on the machine from
// injecting tasks, even if they can reach the socket path.
//
// The check is intentionally UID-only (not PID): PID reuse is a real attack
// surface; UID is stable and sufficient — par-control always runs as the same
// user as par-executor (both are in the same container).
func verifyCaller(conn net.Conn) error {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return fmt.Errorf("verifyCaller: connection is not a UnixConn")
	}

	rawConn, err := unixConn.SyscallConn()
	if err != nil {
		return fmt.Errorf("verifyCaller: SyscallConn: %w", err)
	}

	var ucred *unix.Ucred
	var credErr error
	if ctlErr := rawConn.Control(func(fd uintptr) {
		ucred, credErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); ctlErr != nil {
		return fmt.Errorf("verifyCaller: Control: %w", ctlErr)
	}
	if credErr != nil {
		return fmt.Errorf("verifyCaller: SO_PEERCRED: %w", credErr)
	}

	expectedUID := uint32(os.Getuid()) //nolint:gosec
	if ucred.Uid != expectedUID {
		return fmt.Errorf("verifyCaller: rejected connection from UID %d (expected %d)",
			ucred.Uid, expectedUID)
	}
	return nil
}
