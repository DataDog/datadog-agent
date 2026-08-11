// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package daemon

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

const (
	statusSocketName = "installer-status.sock"

	// statusSocketMode gives the owning group — the Agent user's group — the write
	// permission connecting to a unix socket requires, and nothing to anyone else.
	// Same mode as system-probe's socket (pkg/system-probe/api/server/listener_unix.go).
	statusSocketMode = 0720
)

// umaskMu serializes the umask swap in listenStatusSocket. umask is per-process,
// so any file another goroutine creates while it is swapped would pick it up.
var umaskMu sync.Mutex

// NewStatusAPI returns a new StatusAPI.
//
// The daemon runs as root while the Agent runs as dd-agent, so — unlike the
// local API's 0700 socket — this one has to be reachable by the Agent user.
func NewStatusAPI(daemon Daemon) (StatusAPI, error) {
	return newStatusAPI(daemon, statusSocketPath())
}

func statusSocketPath() string {
	return filepath.Join(paths.RunPath, statusSocketName)
}

func newStatusAPI(daemon statusProvider, socketPath string) (StatusAPI, error) {
	listener, err := listenStatusSocket(socketPath)
	if err != nil {
		return nil, err
	}

	return &statusAPIImpl{
		server:   &http.Server{},
		listener: listener,
		daemon:   daemon,
	}, nil
}

// listenStatusSocket binds the status socket and opens it to the Agent user's group.
//
// paths.RunPath is owned by dd-agent and mode 0755 (see the run directory in
// pkg/fleet/installer/packages/datadog_agent_linux.go), so every step below has to
// assume an unprivileged process can create, replace or unlink entries in that
// directory while we work. That is what makes this different from system-probe,
// whose otherwise identical recipe operates in a root-owned directory:
//
//   - Whatever sits at the path is removed, socket or not. Refusing to unlink a
//     non-socket would let any dd-agent process keep the daemon from starting by
//     planting a regular file there.
//   - The mode is set through the umask at bind time instead of with a chmod
//     afterwards. chmod takes a path, and by the time we called it the path could be
//     a symlink to a file of the attacker's choosing. There is no fd-based
//     alternative: fchmod on a socket fd fails with EINVAL.
//   - The group is set with Lchown, which does not follow symlinks, and only the
//     group changes so the socket stays owned by root.
func listenStatusSocket(socketPath string) (net.Listener, error) {
	if err := os.RemoveAll(socketPath); err != nil {
		return nil, fmt.Errorf("could not remove existing status socket: %w", err)
	}

	listener, err := listenWithMode(socketPath, statusSocketMode)
	if err != nil {
		return nil, err
	}

	perms, err := filesystem.NewPermission()
	if err != nil {
		listener.Close()
		return nil, err
	}
	// A no-op when the Agent user does not exist, which is the right behaviour on a
	// host running the installer without an Agent.
	if err := perms.RestrictGroupAccessNoFollow(socketPath); err != nil {
		listener.Close()
		return nil, fmt.Errorf("error setting status socket group: %w", err)
	}

	return listener, nil
}

// listenWithMode binds a unix socket with the given mode, set at creation time.
func listenWithMode(socketPath string, mode os.FileMode) (net.Listener, error) {
	umaskMu.Lock()
	defer umaskMu.Unlock()

	previous := syscall.Umask(int(^mode & 0777))
	defer syscall.Umask(previous)

	return net.Listen("unix", socketPath)
}

// NewStatusAPIClient returns a new StatusAPIClient.
func NewStatusAPIClient() StatusAPIClient {
	return newStatusAPIClient(statusSocketPath())
}

func newStatusAPIClient(socketPath string) StatusAPIClient {
	return &statusAPIClientImpl{
		client: &http.Client{
			Timeout: statusClientTimeout,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
				},
			},
		},
	}
}
