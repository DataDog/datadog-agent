// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package status

import (
	"context"
	"net"
	"net/http"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

const socketName = "installer-status.sock"

// Address returns the path of the unix socket the daemon serves the status API on.
func Address() string {
	return filepath.Join(paths.RunPath, socketName)
}

// NewClient returns a client for the installer daemon's status API listening on
// addr. Callers outside tests pass Address().
func NewClient(addr string) *Client {
	return &Client{
		client: &http.Client{
			Timeout: defaultTimeout,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, "unix", addr)
				},
			},
		},
	}
}
