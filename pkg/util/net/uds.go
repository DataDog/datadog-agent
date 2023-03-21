// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || darwin
// +build linux darwin

package net

import (
	"net"
	"syscall"
)

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
		return true, nil
	}
	return false, nil
}
