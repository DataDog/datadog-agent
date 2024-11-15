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
)

const (
	socketName = "installer.sock"
)

// NewLocalAPI returns a new LocalAPI.
func NewLocalAPI(daemon Daemon, runPath string) (LocalAPI, error) {
	socketPath := filepath.Join(runPath, socketName)
	err := os.RemoveAll(socketPath)
	if err != nil {
		return nil, fmt.Errorf("could not remove socket: %w", err)
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(socketPath, 0700); err != nil {
		return nil, fmt.Errorf("error setting socket permissions: %v", err)
	}
	return &localAPIImpl{
		server:   &http.Server{},
		listener: listener,
		daemon:   daemon,
	}, nil
}

// NewLocalAPIClient returns a new LocalAPIClient.
func NewLocalAPIClient(runPath string) LocalAPIClient {
	return &localAPIClientImpl{
		addr: "daemon", // this has no meaning when using a unix socket
		client: &http.Client{
			Transport: &http.Transport{
				Dial: func(_, _ string) (net.Conn, error) {
					return net.Dial("unix", filepath.Join(runPath, socketName))
				},
			},
		},
	}
}
