// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !unix && !windows

package client

import (
	"context"
	"net"
	"time"
)

const (
	idleConnTimeout = 30 * time.Second
)

// DialContextFunc is not supported on this platform.
func DialContextFunc(_ string) func(context.Context, string, string) (net.Conn, error) {
	return func(_ context.Context, _, _ string) (net.Conn, error) {
		return nil, ErrNotImplemented
	}
}
