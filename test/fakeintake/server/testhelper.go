// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package server

import (
	"testing"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/require"
)

// InitialiseForTests starts a server with a mock clock and waits for it to be ready.
// It returns the mock clock and the server. Use defer server.Stop() to stop the server
// after calling this function.
func InitialiseForTests(t *testing.T) (*Server, *clock.Mock) {
	t.Helper()
	ready := make(chan bool, 1)
	mockClock := clock.NewMock()
	fi := NewServer(WithReadyChannel(ready), WithClock(mockClock), WithAddress("127.0.0.1:0"))
	fi.Start()
	isReady := <-ready
	require.True(t, isReady)
	return fi, mockClock
}
