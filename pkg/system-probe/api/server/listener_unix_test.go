// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build unix

package server

import (
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testSocketExistsNewUDSListener(t *testing.T, socketPath string) {
	// Pre-create a socket
	_, err := net.Listen("unix", socketPath)
	require.NoError(t, err)

	// Create a new socket using UDSListener
	l, err := NewListener(socketPath)
	require.NoError(t, err)
	_ = l.Close()
}

func testSocketExistsAsRegularFileNewUDSListener(t *testing.T, socketPath string) {
	// Pre-create a file
	f, err := os.OpenFile(socketPath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	require.NoError(t, err)
	defer f.Close()

	// Create a new socket using UDSListener
	_, err = NewListener(socketPath)
	require.Error(t, err)
}

func testWorkingNewUDSListener(t *testing.T, socketPath string) {
	s, err := NewListener(socketPath)
	require.NoError(t, err)
	require.NotNil(t, s)
	defer s.Close()

	time.Sleep(1 * time.Second)
	fi, err := os.Stat(socketPath)
	require.NoError(t, err)
	assert.Equal(t, "Srwx-w----", fi.Mode().String())
}

func TestNewUDSListener(t *testing.T) {
	t.Run("socket exists", func(t *testing.T) {
		testSocketExistsNewUDSListener(t, t.TempDir()+"/net.sock")
	})
	t.Run("non socket exists", func(t *testing.T) {
		testSocketExistsAsRegularFileNewUDSListener(t, t.TempDir()+"/net.sock")
	})
	t.Run("working", func(t *testing.T) {
		testWorkingNewUDSListener(t, t.TempDir()+"/net.sock")
	})
}
