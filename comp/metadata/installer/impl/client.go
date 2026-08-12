// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installerimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	statusapi "github.com/DataDog/datadog-agent/comp/updater/statusapi/def"
)

const (
	// clientTimeout bounds a single status request. The caller is a metadata
	// collector on a timer, so failing fast and reporting the installer as
	// unreachable is better than blocking the collection.
	clientTimeout = 5 * time.Second

	// maxResponseSize bounds how much we are willing to decode from the daemon.
	maxResponseSize = 1 << 20
)

// statusClient reads the installer daemon's read-only status API.
//
// Every request is bounded by clientTimeout; the context is there for a caller that
// wants to give up sooner, not to supply the deadline.
type statusClient interface {
	Status(ctx context.Context) (statusapi.Status, error)
}

type httpStatusClient struct {
	client *http.Client
}

// newStatusClient builds a client over the platform's status transport — a unix
// socket, or a named pipe on Windows. Keep-alive is off: the only caller polls once
// every few minutes, so a pooled connection would sit idle holding an fd and a read
// loop on both ends for nothing.
func newStatusClient(endpoint string) statusClient {
	return &httpStatusClient{
		client: &http.Client{
			Timeout: clientTimeout,
			Transport: &http.Transport{
				DisableKeepAlives: true,
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return dialStatus(ctx, endpoint)
				},
			},
		},
	}
}

// Status returns the current status of the installer daemon.
func (c *httpStatusClient) Status(ctx context.Context) (statusapi.Status, error) {
	var status statusapi.Status
	// The host part is meaningless over a unix socket / named pipe.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://installer/status", nil)
	if err != nil {
		return status, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return status, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return status, fmt.Errorf("installer status API returned %s", resp.Status)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseSize)).Decode(&status); err != nil {
		return status, fmt.Errorf("could not decode installer status: %w", err)
	}
	return status, nil
}
