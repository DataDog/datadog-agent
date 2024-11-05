// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package net

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
)

const (
	connectionsURL       = "http://localhost:3333/" + string(sysconfig.NetworkTracerModule) + "/connections"
	networkIDURL         = "http://unix/" + string(sysconfig.NetworkTracerModule) + "/network_id"
	registerURL          = "http://localhost:3333/" + string(sysconfig.NetworkTracerModule) + "/register"
	languageDetectionURL = "http://localhost:3333/" + string(sysconfig.LanguageDetectionModule) + "/detect"
	statsURL             = "http://localhost:3333/debug/stats"
	pprofURL             = "http://localhost:3333/debug/pprof"
	tracerouteURL        = "http://localhost:3333/" + string(sysconfig.TracerouteModule) + "/traceroute/"
	netType              = "tcp"
	telemetryURL         = "http://localhost:3333/telemetry"

	// discovery* is not used on Windows, the value is added to avoid a compilation error
	discoveryServicesURL = "http://localhost:3333/" + string(sysconfig.DiscoveryModule) + "/services"
	// procStatsURL is not used in windows, the value is added to avoid compilation error in windows
	procStatsURL = "http://localhost:3333/" + string(sysconfig.ProcessModule) + "stats"
	// pingURL is not used in windows, the value is added to avoid compilation error in windows
	pingURL = "http://localhost:3333/" + string(sysconfig.PingModule) + "/ping/"
	// conntrackCachedURL is not used on Windows, the value is added to avoid a compilation error
	conntrackCachedURL = "http://localhost:3333/" + string(sysconfig.NetworkTracerModule) + "/debug/conntrack/cached"
	// conntrackHostURL is not used on Windows, the value is added to avoid a compilation error
	conntrackHostURL = "http://localhost:3333/" + string(sysconfig.NetworkTracerModule) + "/debug/conntrack/host"

	// SystemProbePipeName is the production named pipe for system probe
	SystemProbePipeName = `\\.\pipe\dd_system_probe`

	// systemProbeMaxIdleConns sets the maximum number of idle named pipe connections.
	systemProbeMaxIdleConns = 2

	// systemProbeIdleConnTimeout is the time a named pipe connection is held up idle before being closed.
	// This should be small since connections are local, to close them as soon as they are done,
	// and to quickly service new pending connections.
	systemProbeIdleConnTimeout = 5 * time.Second
)

// CheckPath is used to make sure the globalSocketPath has been set before attempting to connect
func CheckPath(path string) error {
	if path == "" {
		return fmt.Errorf("socket path is empty")
	}
	return nil
}

// NewSystemProbeClient returns a http client configured to talk to the system-probe
func NewSystemProbeClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:    systemProbeMaxIdleConns,
			IdleConnTimeout: systemProbeIdleConnTimeout,
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return DialSystemProbe()
			},
			TLSHandshakeTimeout:   1 * time.Second,
			ResponseHeaderTimeout: 2 * time.Second,
			ExpectContinueTimeout: 50 * time.Millisecond,
		},
	}
}

// newSystemProbe creates a group of clients to interact with system-probe.
func newSystemProbe(path string) *RemoteSysProbeUtil {
	return &RemoteSysProbeUtil{
		path:       path,
		httpClient: *NewSystemProbeClient(),
		pprofClient: http.Client{
			Transport: &http.Transport{
				MaxIdleConns:    systemProbeMaxIdleConns,
				IdleConnTimeout: systemProbeIdleConnTimeout,
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return DialSystemProbe()
				},
			},
		},
		tracerouteClient: http.Client{
			// no timeout set here, the expected usage of this client
			// is that the caller will set a timeout on each request
			Transport: &http.Transport{
				MaxIdleConns:    systemProbeMaxIdleConns,
				IdleConnTimeout: systemProbeIdleConnTimeout,
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return DialSystemProbe()
				},
			},
		},
	}
}

// GetBTFLoaderInfo is not implemented on windows
func (r *RemoteSysProbeUtil) GetBTFLoaderInfo() ([]byte, error) {
	return nil, errors.New("GetBTFLoaderInfo is not supported on windows")
}
