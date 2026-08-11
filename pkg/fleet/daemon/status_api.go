// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/api/middleware"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// statusClientTimeout bounds a single status request. The caller is a metadata
	// collector on a timer, so failing fast and reporting the installer as
	// unreachable is better than blocking the collection.
	statusClientTimeout = 5 * time.Second

	// statusMaxResponseSize bounds how much we are willing to decode from the daemon.
	statusMaxResponseSize = 1 << 20
)

// StatusAPI is the read-only API the daemon exposes to the Agent user.
//
// It is intentionally a second listener rather than a route on LocalAPI: the
// local API's other routes install, remove and promote packages as root, so its
// socket is 0700 and must stay that way. Only add non-sensitive, read-only
// routes here — in particular never serve secrets_pub_key or anything from
// GetRemoteConfigState.
type StatusAPI interface {
	Start(context.Context) error
	Stop(context.Context) error
}

// StatusAPIResponse is the payload returned by the status API.
//
// Only add non-sensitive, read-only fields here: this is served over a socket
// the Agent user can read.
type StatusAPIResponse struct {
	// InstallerVersion is the version of the running installer daemon.
	InstallerVersion string `json:"installer_version"`
	// AvailableDiskSpace is the free space, in bytes, on the partition holding
	// the packages directory. It is nil when the daemon could not determine it —
	// distinguishing "unknown" from a genuine zero matters, because zero free
	// bytes is a real precondition failure.
	AvailableDiskSpace *uint64 `json:"available_disk_space,omitempty"`
}

// statusProvider is the slice of the daemon the status API is allowed to reach.
// Narrowing it here keeps the privileged parts of Daemon out of reach of a
// listener the Agent user can talk to.
type statusProvider interface {
	GetStatus() StatusAPIResponse
}

type statusAPIImpl struct {
	daemon   statusProvider
	listener net.Listener
	server   *http.Server
}

// Start starts the StatusAPI.
func (s *statusAPIImpl) Start(_ context.Context) error {
	s.server.Handler = s.handler()
	go func() {
		err := s.server.Serve(s.listener)
		if err != nil {
			log.Infof("Installer status API server stopped: %v", err)
		}
	}()
	return nil
}

// Stop stops the StatusAPI.
func (s *statusAPIImpl) Stop(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *statusAPIImpl) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", s.status)
	return middleware.RequireContentType("application/json")(mux)
}

// example: curl --unix-socket /opt/datadog-packages/run/installer-status.sock -H 'Content-Type: application/json' http://installer/status
func (s *statusAPIImpl) status(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(s.daemon.GetStatus()); err != nil {
		log.Warnf("could not write installer status response: %v", err)
	}
}

// StatusAPIClient reads the daemon's read-only status API.
//
// Unlike LocalAPIClient this takes a context: its caller is the Agent's metadata
// collector rather than a CLI command, so it needs to bound the request with its
// own deadline instead of inheriting the client's.
type StatusAPIClient interface {
	Status(ctx context.Context) (StatusAPIResponse, error)
}

type statusAPIClientImpl struct {
	client *http.Client
}

// Status returns the current status of the installer daemon.
func (c *statusAPIClientImpl) Status(ctx context.Context) (StatusAPIResponse, error) {
	var response StatusAPIResponse
	// The host part is meaningless over a unix socket / named pipe.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://installer/status", nil)
	if err != nil {
		return response, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return response, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return response, fmt.Errorf("installer status API returned %s", resp.Status)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, statusMaxResponseSize)).Decode(&response); err != nil {
		return response, fmt.Errorf("could not decode installer status: %w", err)
	}
	return response, nil
}
