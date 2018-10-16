// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build linux
// Origin detection is linux-only

// Most of it is tested by test/integration/dogstatsd/origin_detection_test.go
// that requires a docker environment to run

package listeners

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
	"golang.org/x/sys/unix"
)

func TestUDSPassCred(t *testing.T) {
	dir, err := ioutil.TempDir("", "dd-test-")
	assert.Nil(t, err)
	defer os.RemoveAll(dir) // clean up
	socketPath := filepath.Join(dir, "dsd.socket")

	mockConfig := config.NewMock()
	mockConfig.Set("dogstatsd_socket", socketPath)
	mockConfig.Set("dogstatsd_origin_detection", true)

	packetPoolUDS := NewPacketPool(config.Datadog.GetInt("dogstatsd_buffer_size"))
	s, err := NewUDSListener(nil, packetPoolUDS)
	defer s.Stop()

	assert.Nil(t, err)
	assert.NotNil(t, s)

	// Test socket has PASSCRED option set to 1
	f, err := s.conn.File()
	defer f.Close()
	assert.Nil(t, err)

	enabled, err := unix.GetsockoptInt(int(f.Fd()), unix.SOL_SOCKET, unix.SO_PASSCRED)
	assert.Nil(t, err)
	assert.Equal(t, enabled, 1)
}
