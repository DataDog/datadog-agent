// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package net

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
	addr, err := net.ResolveUnixAddr("unix", socketPath)
	assert.NoError(t, err)
	_, err = net.Listen("unix", addr.Name)
	assert.NoError(t, err)

	// Create a new socket using UDSListener
	l, err := NewListener(socketPath)
	require.NoError(t, err)

	l.Stop()
}

func testSocketExistsAsRegularFileNewUDSListener(t *testing.T, socketPath string) {
	// Pre-create a file
	f, err := os.OpenFile(socketPath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	assert.NoError(t, err)
	defer f.Close()

	// Create a new socket using UDSListener
	_, err = NewListener(socketPath)
	require.Error(t, err)
}

func testWorkingNewUDSListener(t *testing.T, socketPath string) {
	s, err := NewListener(socketPath)
	require.NoError(t, err)
	defer s.Stop()

	assert.NoError(t, err)
	assert.NotNil(t, s)
	time.Sleep(1 * time.Second)
	fi, err := os.Stat(socketPath)
	require.NoError(t, err)
	assert.Equal(t, "Srwx-w----", fi.Mode().String())
}

func TestNewUDSListener(t *testing.T) {
	t.Run("socket_exists_but_is_successfully_removed", func(tt *testing.T) {
		dir := t.TempDir()
		testSocketExistsNewUDSListener(tt, dir+"/net.sock")
	})

	t.Run("non_socket_exists_and_fails_to_be_removed", func(tt *testing.T) {
		dir := t.TempDir()
		testSocketExistsAsRegularFileNewUDSListener(tt, dir+"/net.sock")
	})

	t.Run("working", func(tt *testing.T) {
		dir := t.TempDir()
		testWorkingNewUDSListener(tt, dir+"/net.sock")
	})
}
