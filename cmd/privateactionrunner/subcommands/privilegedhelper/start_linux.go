// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package privilegedhelper

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/user"
	"runtime"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

const (
	// minLandlockABI is the Landlock ABI the helper sandbox requires.
	minLandlockABI = 3
	// socketOwner is the only account allowed to connect to the helper.
	socketOwner = "dd-agent"
)

// helperTrampoline gives the helper the socket the way systemd socket
// activation does: on fd 3, with LISTEN_PID set to the helper's own PID. The
// shell keeps this process's PID across both execs, so systemd keeps
// supervising the helper as the service's main process. Moving the
// descriptor in Go instead could overwrite one the Go runtime owns.
const helperTrampoline = `exec 3<&%[1]d %[1]d<&-; LISTEN_PID=$$ LISTEN_FDS=1 exec "$@"`

var (
	landlockABIVersion = func() (int, error) {
		// Querying the ABI version needs no privileges.
		version, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, unix.LANDLOCK_CREATE_RULESET_VERSION)
		if errno != 0 {
			return 0, errno
		}
		return int(version), nil
	}
	execve     = syscall.Exec
	lookupUser = user.Lookup
)

func startHelper(logger log.Component, params *helperParams) error {
	if params.helperPath == "" {
		return errors.New("--helper is required")
	}
	if version, err := landlockABIVersion(); err != nil || version < minLandlockABI {
		logger.Warnf("not starting the rshell privileged helper: it needs Landlock ABI %d (got %d, %v)", minLandlockABI, version, err)
		return nil
	}
	account, err := lookupUser(socketOwner)
	if err != nil {
		return fmt.Errorf("look up %s: %w", socketOwner, err)
	}
	uid, err := strconv.Atoi(account.Uid)
	if err != nil {
		return fmt.Errorf("parse %s uid: %w", socketOwner, err)
	}
	gid, err := strconv.Atoi(account.Gid)
	if err != nil {
		return fmt.Errorf("parse %s gid: %w", socketOwner, err)
	}
	socket, err := listen(params.socketPath, uid, gid)
	if err != nil {
		return err
	}
	logger.Infof("starting the rshell privileged helper on %s", params.socketPath)
	args := []string{"sh", "-c", fmt.Sprintf(helperTrampoline, socket.Fd()), "sh",
		params.helperPath, "privileged-helper", "--policy=" + params.policyPath, "--idle-timeout=0"}
	// The helper runs commands with an empty environment; pass it nothing
	// from the Agent's, which can hold credentials.
	err = execve("/bin/sh", args, []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin"})
	// Only reached if exec failed: until then, the socket must not be closed
	// by its finalizer.
	runtime.KeepAlive(socket)
	return fmt.Errorf("exec the rshell privileged helper: %w", err)
}

// listen creates the helper socket, readable and writable by uid only, and
// returns its descriptor, kept open across exec.
func listen(path string, uid, gid int) (*os.File, error) {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("remove stale socket: %w", err)
	}
	oldUmask := unix.Umask(0o177)
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	unix.Umask(oldUmask)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", path, err)
	}
	// Closing the listener would otherwise unlink the socket the helper serves.
	listener.SetUnlinkOnClose(false)
	if err := os.Chown(path, uid, gid); err != nil {
		listener.Close()
		return nil, fmt.Errorf("chown %s: %w", path, err)
	}
	// File returns a duplicate descriptor marked close-on-exec.
	socket, err := listener.File()
	if err != nil {
		listener.Close()
		return nil, fmt.Errorf("get socket descriptor: %w", err)
	}
	if _, err := unix.FcntlInt(socket.Fd(), unix.F_SETFD, 0); err != nil {
		listener.Close()
		socket.Close()
		return nil, fmt.Errorf("keep socket open across exec: %w", err)
	}
	return socket, nil
}
