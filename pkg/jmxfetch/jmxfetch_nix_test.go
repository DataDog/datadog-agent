// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build jmx && !windows

package jmxfetch

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestGetPreferredDSDEndpointNix(t *testing.T) {
	cfg := configmock.New(t)

	// Helper to create a unixgram socket at a short path under /tmp
	// (macOS has a 108-char limit on Unix socket paths).
	createSocket := func(t *testing.T) string {
		t.Helper()
		dir, err := os.MkdirTemp("/tmp", "dsd-test-*")
		require.NoError(t, err)
		t.Cleanup(func() { os.RemoveAll(dir) })
		sockPath := filepath.Join(dir, "s")
		conn, err := net.ListenPacket("unixgram", sockPath)
		require.NoError(t, err)
		t.Cleanup(func() { conn.Close() })
		return sockPath
	}

	t.Run("UDS available", func(t *testing.T) {
		sockPath := createSocket(t)
		cfg.SetWithoutSource("dogstatsd_socket", sockPath)
		cfg.SetWithoutSource("use_dogstatsd", true)

		j := NewJMXFetch(nil, nil)
		assert.Equal(t, "statsd:unix://"+sockPath, j.getPreferredDSDEndpoint())
	})

	t.Run("DSD disabled falls back to UDP", func(t *testing.T) {
		sockPath := createSocket(t)
		cfg.SetWithoutSource("dogstatsd_socket", sockPath)
		cfg.SetWithoutSource("dogstatsd_port", "8125")
		cfg.SetWithoutSource("use_dogstatsd", false)

		j := NewJMXFetch(nil, nil)
		assert.Equal(t, "statsd:localhost:8125", j.getPreferredDSDEndpoint())
	})
}
