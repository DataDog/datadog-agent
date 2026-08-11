// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package daemon

import (
	"fmt"
	"net"
	"net/http"
	"os"

	daemonstatus "github.com/DataDog/datadog-agent/pkg/fleet/daemon/status"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

// NewStatusAPI returns a new StatusAPI.
//
// The daemon runs as root while the Agent runs as dd-agent, so — unlike the
// local API's 0700 socket — this one has to be reachable by the Agent user. We
// use the same recipe as system-probe (pkg/system-probe/api/server/listener_unix.go):
// 0720 gives the owning group connect rights, and RestrictAccessToUser chowns the
// socket to dd-agent (_dd-agent on macOS). That call is a no-op when the Agent
// user does not exist, which is the right behaviour on an installer-only host.
func NewStatusAPI(daemon Daemon) (StatusAPI, error) {
	return newStatusAPI(daemon, daemonstatus.Address())
}

func newStatusAPI(daemon statusProvider, socketPath string) (StatusAPI, error) {
	// Remove any pre-existing socket, but refuse to unlink something that is not one.
	if fileInfo, err := os.Stat(socketPath); err == nil {
		if fileInfo.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("could not reuse %s: path exists and is not a unix socket", socketPath)
		}
		if err := os.Remove(socketPath); err != nil {
			return nil, fmt.Errorf("could not remove stale status socket: %w", err)
		}
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(socketPath, 0720); err != nil {
		return nil, fmt.Errorf("error setting status socket permissions: %w", err)
	}
	perms, err := filesystem.NewPermission()
	if err != nil {
		return nil, err
	}
	if err := perms.RestrictAccessToUser(socketPath); err != nil {
		return nil, err
	}

	return &statusAPIImpl{
		server:   &http.Server{},
		listener: listener,
		daemon:   daemon,
	}, nil
}
