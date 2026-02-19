// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package net

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsUDSAvailable_NonExistentPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "socket-we-wont-ever-create")
	assert.False(t, IsUDSAvailable(path))
}

func TestIsUDSAvailable_RegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-socket")
	require.NoError(t, writeFile(path))
	assert.False(t, IsUDSAvailable(path))
}

func TestIsUDSAvailable_ValidSocket(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "test.sock")
	conn, err := net.ListenPacket("unixgram", sockPath)
	require.NoError(t, err)
	defer conn.Close()

	assert.True(t, IsUDSAvailable(sockPath))
}

// writeFile creates a regular file at path for testing.
func writeFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	return f.Close()
}
