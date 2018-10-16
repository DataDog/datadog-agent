// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !windows
// UDS won't work in windows

package listeners

import (
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
)

var packetPoolUDS = NewPacketPool(config.Datadog.GetInt("dogstatsd_buffer_size"))

func testFileExistsNewUDSListener(t *testing.T, socketPath string) {
	_, err := os.Create(socketPath)
	assert.Nil(t, err)
	defer os.Remove(socketPath)
	_, err = NewUDSListener(nil, packetPoolUDS)
	assert.Error(t, err)
}

func testSocketExistsNewUSDListener(t *testing.T, socketPath string) {
	address, err := net.ResolveUnixAddr("unix", socketPath)
	assert.Nil(t, err)
	_, err = net.ListenUnix("unix", address)
	assert.Nil(t, err)
	testWorkingNewUDSListener(t, socketPath)
}

func testWorkingNewUDSListener(t *testing.T, socketPath string) {
	s, err := NewUDSListener(nil, packetPoolUDS)
	defer s.Stop()

	assert.Nil(t, err)
	assert.NotNil(t, s)
	fi, err := os.Stat(socketPath)
	require.Nil(t, err)
	assert.Equal(t, "Srwx-w--w-", fi.Mode().String())
}

func TestNewUDSListener(t *testing.T) {
	dir, err := ioutil.TempDir("", "dd-test-")
	assert.Nil(t, err)
	defer os.RemoveAll(dir) // clean up
	socketPath := filepath.Join(dir, "dsd.socket")
	mockConfig := config.NewMock()
	mockConfig.Set("dogstatsd_socket", socketPath)

	t.Run("fail_file_exists", func(tt *testing.T) {
		testFileExistsNewUDSListener(tt, socketPath)
	})
	t.Run("socket_exists", func(tt *testing.T) {
		testSocketExistsNewUSDListener(tt, socketPath)
	})
	t.Run("working", func(tt *testing.T) {
		testWorkingNewUDSListener(tt, socketPath)
	})
}

func TestStartStopUDSListener(t *testing.T) {
	dir, err := ioutil.TempDir("", "dd-test-")
	assert.Nil(t, err)
	defer os.RemoveAll(dir) // clean up
	socketPath := filepath.Join(dir, "dsd.socket")

	mockConfig := config.NewMock()
	mockConfig.Set("dogstatsd_socket", socketPath)
	mockConfig.Set("dogstatsd_origin_detection", false)
	s, err := NewUDSListener(nil, packetPoolUDS)
	assert.Nil(t, err)
	assert.NotNil(t, s)

	go s.Listen()
	conn, err := net.Dial("unixgram", socketPath)
	assert.Nil(t, err)
	conn.Close()

	s.Stop()
	conn, err = net.Dial("unixgram", socketPath)
	assert.NotNil(t, err)
}

func TestUDSReceive(t *testing.T) {
	dir, err := ioutil.TempDir("", "dd-test-")
	assert.Nil(t, err)
	defer os.RemoveAll(dir) // clean up
	socketPath := filepath.Join(dir, "dsd.socket")

	mockConfig := config.NewMock()
	mockConfig.Set("dogstatsd_socket", socketPath)
	mockConfig.Set("dogstatsd_origin_detection", false)

	var contents = []byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2")

	packetChannel := make(chan *Packet)
	s, err := NewUDSListener(packetChannel, packetPoolUDS)
	assert.Nil(t, err)
	assert.NotNil(t, s)

	go s.Listen()
	defer s.Stop()
	conn, err := net.Dial("unixgram", socketPath)
	assert.Nil(t, err)
	defer conn.Close()
	conn.Write(contents)

	select {
	case packet := <-packetChannel:
		assert.NotNil(t, packet)
		assert.Equal(t, packet.Contents, contents)
		assert.Equal(t, packet.Origin, "")
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

}
