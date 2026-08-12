// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package filesystem

import (
	"net"
	"os"
	"sync"
	"syscall"
)

// umaskMu serializes the umask swap in ListenUnixSocketWithMode. umask is
// per-process, so any file another goroutine creates while it is swapped would pick
// it up — including one created by a caller in another package, which is why the
// swap lives here rather than being reimplemented next to each listener.
var umaskMu sync.Mutex

// ListenUnixSocketWithMode binds a unix socket with the given mode, set through the
// umask at bind time rather than with a chmod afterwards.
//
// Prefer it for a socket in a directory an unprivileged user can write to: chmod
// takes a path, so between binding the socket and chmod'ing it, a process that can
// write the directory can substitute a symlink and have the privileged process
// change the mode of a file of its choosing. There is no fd-based alternative —
// fchmod on a socket fd fails with EINVAL — which leaves the umask.
func ListenUnixSocketWithMode(socketPath string, mode os.FileMode) (net.Listener, error) {
	umaskMu.Lock()
	defer umaskMu.Unlock()

	previous := syscall.Umask(int(^mode & 0777))
	defer syscall.Umask(previous)

	return net.Listen("unix", socketPath)
}
