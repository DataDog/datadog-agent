// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package status holds the wire format and the client for the installer daemon's
// read-only status API.
//
// This API is separate from the daemon's local API (`installer.sock`) on purpose.
// Every other route of the local API installs, removes or promotes packages as
// root, so that socket is `0700` and must stay that way. This one only exposes
// non-sensitive read-only state, so its socket is opened to the Agent user with
// the same permission recipe system-probe uses to serve `dd-agent`.
//
// The package is deliberately a leaf: the core Agent imports it to read the
// installer status without linking the daemon itself.
package status

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// defaultTimeout bounds a single status request. The caller is a metadata
	// collector on a timer, so failing fast and reporting the installer as
	// unreachable is better than blocking the collection.
	defaultTimeout = 5 * time.Second

	// maxResponseSize bounds how much we are willing to decode from the daemon.
	maxResponseSize = 1 << 20
)

// Response is the payload returned by the installer daemon's status endpoint.
//
// Only add non-sensitive, read-only fields here: this is served over a socket
// the Agent user can read.
type Response struct {
	// InstallerVersion is the version of the running installer daemon.
	InstallerVersion string `json:"installer_version"`
	// AvailableDiskSpace is the free space, in bytes, on the partition holding
	// the packages directory. It is nil when the daemon could not determine it —
	// distinguishing "unknown" from a genuine zero matters, because zero free
	// bytes is a real precondition failure.
	AvailableDiskSpace *uint64 `json:"available_disk_space,omitempty"`
}

// Client reads the installer daemon's status API.
type Client struct {
	client *http.Client
}

// Status returns the current status of the installer daemon.
func (c *Client) Status(ctx context.Context) (*Response, error) {
	// The host part is meaningless over a unix socket / named pipe.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://installer/status", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("installer status API returned %s", resp.Status)
	}
	var response Response
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseSize)).Decode(&response); err != nil {
		return nil, fmt.Errorf("could not decode installer status: %w", err)
	}
	return &response, nil
}
