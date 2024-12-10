// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package net

import (
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
)

const (
	pingURL        = "http://unix/" + string(sysconfig.PingModule) + "/ping/"
	tracerouteURL  = "http://unix/" + string(sysconfig.TracerouteModule) + "/traceroute/"
	connectionsURL = "http://unix/" + string(sysconfig.NetworkTracerModule) + "/connections"
	networkIDURL   = "http://unix/" + string(sysconfig.NetworkTracerModule) + "/network_id"
	procStatsURL   = "http://unix/" + string(sysconfig.ProcessModule) + "/stats"
	registerURL    = "http://unix/" + string(sysconfig.NetworkTracerModule) + "/register"
	statsURL       = "http://unix/debug/stats"
)

// CheckPath is used in conjunction with calling the stats endpoint, since we are calling this
// From the main agent and want to ensure the socket exists
func CheckPath(path string) error {
	if path == "" {
		return errors.New("socket path is empty")
	}

	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("socket path does not exist: %v", err)
	}
	return nil
}

// newSystemProbe creates a group of clients to interact with system-probe.
func newSystemProbe(path string) *RemoteSysProbeUtil {
	return &RemoteSysProbeUtil{
		path:       path,
		httpClient: *client.Get(path),
		tracerouteClient: http.Client{
			// no timeout set here, the expected usage of this client
			// is that the caller will set a timeout on each request
			Transport: &http.Transport{
				DialContext: client.DialContextFunc(path),
			},
		},
	}
}
