// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netlink

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// noallocRecvmsg is a copy of unix.Recvmsg without the allocation from unix.RawSockaddrAny
func noallocRecvmsg(fd int, p, oob []byte, flags int) (n, oobn int, recvflags int, err error) {
	n, oobn, recvflags, err = recvmsgRaw(fd, p, oob, flags)
	return
}

func recvmsgRaw(fd int, p, oob []byte, flags int) (n, oobn int, recvflags int, err error) {
	var msg unix.Msghdr
	var iov unix.Iovec
	if len(p) > 0 {
		iov.Base = &p[0]
		iov.SetLen(len(p))
	}
	var dummy byte
	if len(oob) > 0 {
		if len(p) == 0 {
			var sockType int
			sockType, err = unix.GetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_TYPE)
			if err != nil {
				return
			}
			// receive at least one normal byte
			if sockType != unix.SOCK_DGRAM {
				iov.Base = &dummy
				iov.SetLen(1)
			}
		}
		msg.Control = &oob[0]
		msg.SetControllen(len(oob))
	}
	msg.Iov = &iov
	msg.Iovlen = 1
	if n, err = recvmsg(fd, &msg, flags); err != nil {
		return
	}
	oobn = int(msg.Controllen)
	recvflags = int(msg.Flags)
	return
}

func recvmsg(s int, msg *unix.Msghdr, flags int) (n int, err error) {
	r0, _, e1 := unix.Syscall(unix.SYS_RECVMSG, uintptr(s), uintptr(unsafe.Pointer(msg)), uintptr(flags))
	n = int(r0)
	if e1 != 0 {
		err = errnoErr(e1)
	}
	return
}

// Do the interface allocations only once for common
// Errno values.
var (
	errEAGAIN error = syscall.EAGAIN
	errEINVAL error = syscall.EINVAL
	errENOENT error = syscall.ENOENT
)

// errnoErr returns common boxed Errno values, to prevent
// allocations at runtime.
func errnoErr(e syscall.Errno) error {
	switch e {
	case 0:
		return nil
	case unix.EAGAIN:
		return errEAGAIN
	case unix.EINVAL:
		return errEINVAL
	case unix.ENOENT:
		return errENOENT
	}
	return e
}
