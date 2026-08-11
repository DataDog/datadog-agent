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
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

const (
	socketName = "installer.sock"

	// socketMode keeps the local API root-only: every route on it but /status
	// installs, removes or promotes packages as root.
	socketMode = 0700
)

// NewLocalAPI returns a new LocalAPI.
func NewLocalAPI(daemon Daemon) (LocalAPI, error) {
	socketPath := filepath.Join(paths.RunPath, socketName)
	err := os.RemoveAll(socketPath)
	if err != nil {
		return nil, fmt.Errorf("could not remove socket: %w", err)
	}
	// The mode is set at bind time rather than with a chmod afterwards: this socket
	// lives in a directory dd-agent can write to, so a chmod by path can be pointed
	// at a symlink of that user's choosing. See listenStatusSocket.
	listener, err := listenWithMode(socketPath, socketMode)
	if err != nil {
		return nil, err
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
				Dial: func(_, _ string) (net.Conn, error) {
					return net.Dial("unix", filepath.Join(paths.RunPath, socketName))
				},
			},
		},
	}
}
