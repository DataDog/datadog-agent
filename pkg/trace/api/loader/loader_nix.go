// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || darwin

package loader

import (
	"errors"
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// GetFDFromListener returns a file descriptor for the given listener.
//
// It duplicates the underlying file descriptor so that it's still valid when the file is
// closed or garbage collected.
// The returned file descriptor can be passed to child processes using the exec syscall.
func GetFDFromListener(ln net.Listener) (uintptr, error) {
	// File is not exposed by the listener interface, but it is implemented by
	// net.TCPListener and net.UnixListener.
	lnf, ok := ln.(interface {
		File() (*os.File, error)
	})
	if !ok {
		// This should never happen, but we'll check just in case.
		return 0, errors.New("listener does not support File()")
	}

	// The documentation for File() says:
	//// File returns a copy of the underlying [os.File].
	//// It is the caller's responsibility to close f when finished.
	//// Closing l does not affect f, and closing f does not affect l.
	////
	//// The returned os.File's file descriptor is different from the
	//// connection's. Attempting to change properties of the original
	//// using this duplicate may or may not have the desired effect.
	//
	// So we duplicate the file descriptor and close the file.
	// The descriptor returned by Fd() is managed by the Go runtime, so we can't use it directly
	// (in particular to pass through the exec syscall).

	f, err := lnf.File()
	if err != nil {
		return 0, fmt.Errorf("failed to get file from listener: %v", err)
	}
	defer f.Close()

	origFD := f.Fd()
	// Duplicate the file descriptor so that it's still valid when the file is
	// closed or garbage collected
	duppedFD, err := unix.Dup(int(origFD))
	if err != nil {
		return 0, fmt.Errorf("failed to duplicate file descriptor: %v", err)
	}

	fd := uintptr(duppedFD)
	// Get the current flags of the file descriptor, so that we can check if CLOEXEC is set.
	// If CLOEXEC is set, we remove it so that the file descriptor is not closed when using the exec syscall.
	flag, err := unix.FcntlInt(fd, unix.F_GETFD, 0)
	if err != nil {
		return 0, fmt.Errorf("fcntl GETFD: %v", err)
	}

	if flag&unix.FD_CLOEXEC != 0 {
		_, err := unix.FcntlInt(fd, unix.F_SETFD, flag & ^unix.FD_CLOEXEC)
		if err != nil {
			return 0, fmt.Errorf("fcntl SETFD: %v", err)
		}
	}

	return fd, nil
}
