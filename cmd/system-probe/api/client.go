// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package api contains the API exposed by system-probe
package api

import (
	"context"
	"net"
	"net/http"
	"time"
)

// GetClient returns a http client configured to talk to the system-probe
func GetClient(socketPath string) *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:    2,
			IdleConnTimeout: 30 * time.Second,
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial(netType, socketPath)
			},
			TLSHandshakeTimeout:   1 * time.Second,
			ResponseHeaderTimeout: 5 * time.Second,
			ExpectContinueTimeout: 50 * time.Millisecond,
		},
	}
}
