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
	"runtime"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

const (
	socketName = "installer.sock"
)

// NewLocalAPI returns a new LocalAPI.
func NewLocalAPI(daemon Daemon) (LocalAPI, error) {
	socketPath := filepath.Join(paths.RunPath, socketName)
	err := os.RemoveAll(socketPath)
	if err != nil {
		return nil, fmt.Errorf("could not remove socket: %w", err)
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	// RestrictAccessToUser below only chowns the socket to _dd-agent's own uid/gid, it
	// doesn't touch these mode bits. 0700 would only let the exact chowned owner connect;
	// the group-write bit is what lets other processes that are merely members of
	// _dd-agent's group (not the exact uid) dial the socket. On macOS, dropping it to 0700
	// previously broke `datadog-agent status`, which reported the installer as not running
	// (see 7899fd155ed), so this stays 0720 there. Linux keeps its original 0700: nothing
	// there relies on group-based access, so there's no reason to loosen it.
	socketPerm := os.FileMode(0700)
	if runtime.GOOS == "darwin" {
		socketPerm = 0720
	}
	if err := os.Chmod(socketPath, socketPerm); err != nil {
		return nil, fmt.Errorf("error setting socket permissions: %v", err)
	}
	perms, err := filesystem.NewPermission()
	if err != nil {
		return nil, err
	}
	if err := perms.RestrictAccessToUser(socketPath); err != nil {
		return nil, fmt.Errorf("error restricting socket access: %v", err)
	}
	return &localAPIImpl{
		server:   &http.Server{},
		listener: listener,
		daemon:   daemon,
	}, nil
}

// NewLocalAPIClient returns a new LocalAPIClient.
func NewLocalAPIClient() LocalAPIClient {
	return &localAPIClientImpl{
		addr: "daemon", // this has no meaning when using a unix socket
		client: &http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", filepath.Join(paths.RunPath, socketName))
				},
			},
		},
	}
}
