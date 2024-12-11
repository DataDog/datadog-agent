// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netlink

import (
	"syscall"

	"github.com/mdlayher/netlink"
	"github.com/mdlayher/netlink/nlenc"
)

// Copyright (C) 2016-2021 Matt Layher
// Source https://github.com/mdlayher/netlink/blob/ec511443387bb32b3adcd448828476c83754c8e8/message.go
// checkMessage checks a single Message for netlink errors.
func checkMessage(m netlink.Message) error {
	const success = 0

	// Per libnl documentation, only messages that indicate type error can
	// contain error codes:
	// https://www.infradead.org/~tgr/libnl/doc/core.html#core_errmsg.
	//
	// However, at one point, this package checked both done and error for
	// error codes.  Because there was no issue associated with the change,
	// it is unknown whether this change was correct or not.  If you run into
	// a problem with your application because of this change, please file
	// an issue.
	if m.Header.Type != netlink.Error {
		return nil
	}

	if len(m.Data) < 4 {
		return errShortErrorMessage
	}

	if c := nlenc.Int32(m.Data[0:4]); c != success {
		// Error code is a negative integer, convert it into an OS-specific raw
		// system call error, but do not wrap with os.NewSyscallError to signify
		// that this error was produced by a netlink message; not a system call.
		return syscall.Errno(-1 * int(c))
	}

	return nil
}
