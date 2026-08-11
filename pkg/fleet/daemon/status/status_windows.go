// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package status

import (
	"context"
	"net"
	"net/http"

	"github.com/Microsoft/go-winio"
)

const namedPipePath = `\\.\pipe\DD_INSTALLER_STATUS`

// Address returns the named pipe the daemon serves the status API on.
func Address() string {
	return namedPipePath
}

// NewClient returns a client for the installer daemon's status API listening on
// addr. Callers outside tests pass Address().
func NewClient(addr string) *Client {
	return &Client{
		client: &http.Client{
			Timeout: defaultTimeout,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return winio.DialPipeContext(ctx, addr)
				},
			},
		},
	}
}
