// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build unix

package client

import (
	"context"
	"net"
	"time"
)

const (
	idleConnTimeout = 30 * time.Second
)

// DialContextFunc returns a function to be used in http.Transport.DialContext for connecting to system-probe.
// The result will be OS-specific.
func DialContextFunc(socketPath string) func(context.Context, string, string) (net.Conn, error) {
	return func(_ context.Context, _, _ string) (net.Conn, error) {
		return net.Dial("unix", socketPath)
	}
}
