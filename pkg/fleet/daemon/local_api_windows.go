// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package daemon

import (
	"context"
	"github.com/Microsoft/go-winio"
	"net"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/fleet/internal/paths"
)

const (
	namedPipePath = "\\\\.\\pipe\\DD_INSTALLER"
)

// NewLocalAPI returns a new LocalAPI.
func NewLocalAPI(daemon Daemon, _ string) (LocalAPI, error) {
	// Prevent daemon from running in insecure directories
	err := paths.IsInstallerDataDirSecure()
	if err != nil {
		return nil, err
	}
	listener, err := winio.ListenPipe(namedPipePath, &winio.PipeConfig{
		SecurityDescriptor: "D:P(A;;GA;;;SY)(A;;GA;;;BA)",
		MessageMode:        false,
	})
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
func NewLocalAPIClient(_ string) LocalAPIClient {
	return &localAPIClientImpl{
		addr: "daemon",
		client: &http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					// Default timeout is 2s
					return winio.DialPipe(namedPipePath, nil)
				},
			},
		},
	}
}
