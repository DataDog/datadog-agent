// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

// Origin detection is linux-only

// Most of it is tested by test/integration/dogstatsd/origin_detection_test.go
// that requires a docker environment to run

package listeners

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd/packets"
)

func TestUDSPassCred(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "dsd.socket")

	mockConfig := config.Mock(t)
	mockConfig.Set("dogstatsd_socket", socketPath)
	mockConfig.Set("dogstatsd_origin_detection", true)

	pool := packets.NewPool(512)
	poolManager := packets.NewPoolManager(pool)
	s, err := NewUDSListener(nil, poolManager, nil)
	defer s.Stop()

	assert.Nil(t, err)
	assert.NotNil(t, s)

	// Test socket has PASSCRED option set to 1
	f, err := s.conn.File()
	require.Nil(t, err)
	defer f.Close()

	enabled, err := unix.GetsockoptInt(int(f.Fd()), unix.SOL_SOCKET, unix.SO_PASSCRED)
	assert.Nil(t, err)
	assert.Equal(t, enabled, 1)
}
