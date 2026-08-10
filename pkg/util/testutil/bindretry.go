// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package testutil

import (
	"errors"
	"net"
	"runtime"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

// MaxBindAttempts bounds retries when a freshly-picked port loses the bind
// race to something else between being picked and the real server binding it.
const MaxBindAttempts = 5

// wsaeAddrInUse is WSAEADDRINUSE, the raw Winsock error code Go's net
// package passes through unmodified on Windows binds; it doesn't map to the
// stdlib syscall.EADDRINUSE constant, which is a different synthetic value.
const wsaeAddrInUse = syscall.Errno(10048)

// IsAddrInUse reports whether err is a failure to bind because the port was
// already taken.
func IsAddrInUse(err error) bool {
	if runtime.GOOS == "windows" {
		return errors.Is(err, wsaeAddrInUse)
	}
	return errors.Is(err, syscall.EADDRINUSE)
}

// FreeTCPPort asks the OS for a currently-free TCP port.
func FreeTCPPort(t testing.TB) int {
	t.Helper()
	l, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
