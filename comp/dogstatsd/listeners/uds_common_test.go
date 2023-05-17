// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// UDS won't work in windows

package listeners

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
)

func newPacketPoolManagerUDS(cfg config.Component) *packets.PoolManager {
	packetPoolUDS := packets.NewPool(cfg.GetInt("dogstatsd_buffer_size"))
	return packets.NewPoolManager(packetPoolUDS)
}

func testFileExistsNewUDSListener(t *testing.T, socketPath string, cfg map[string]interface{}) {
	_, err := os.Create(socketPath)
	assert.Nil(t, err)
	defer os.Remove(socketPath)
	config := fulfillDepsWithConfig(t, cfg)
	_, err = NewUDSListener(nil, newPacketPoolManagerUDS(config), config, nil)
	assert.Error(t, err)
}

func testSocketExistsNewUSDListener(t *testing.T, socketPath string, cfg map[string]interface{}) {
	address, err := net.ResolveUnixAddr("unix", socketPath)
	assert.Nil(t, err)
	_, err = net.ListenUnix("unix", address)
	assert.Nil(t, err)
	testWorkingNewUDSListener(t, socketPath, cfg)
}

func testWorkingNewUDSListener(t *testing.T, socketPath string, cfg map[string]interface{}) {
	config := fulfillDepsWithConfig(t, cfg)
	s, err := NewUDSListener(nil, newPacketPoolManagerUDS(config), config, nil)
	defer s.Stop()

	assert.Nil(t, err)
	assert.NotNil(t, s)
	fi, err := os.Stat(socketPath)
	require.Nil(t, err)
	assert.Equal(t, "Srwx-w--w-", fi.Mode().String())
}

func TestNewUDSListener(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "dsd.socket")
	mockConfig := map[string]interface{}{}
	mockConfig["dogstatsd_socket"] = socketPath

	t.Run("fail_file_exists", func(tt *testing.T) {
		testFileExistsNewUDSListener(tt, socketPath, mockConfig)
	})
	t.Run("socket_exists", func(tt *testing.T) {
		testSocketExistsNewUSDListener(tt, socketPath, mockConfig)
	})
	t.Run("working", func(tt *testing.T) {
		testWorkingNewUDSListener(tt, socketPath, mockConfig)
	})
}

func TestStartStopUDSListener(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "dsd.socket")

	mockConfig := map[string]interface{}{}
	mockConfig["dogstatsd_socket"] = socketPath
	mockConfig["dogstatsd_origin_detection"] = false

	config := fulfillDepsWithConfig(t, mockConfig)
	s, err := NewUDSListener(nil, newPacketPoolManagerUDS(config), config, nil)
	assert.Nil(t, err)
	assert.NotNil(t, s)

	go s.Listen()
	conn, err := net.Dial("unixgram", socketPath)
	assert.Nil(t, err)
	conn.Close()

	s.Stop()
	_, err = net.Dial("unixgram", socketPath)
	assert.NotNil(t, err)
}

func TestUDSReceive(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "dsd.socket")

	mockConfig := map[string]interface{}{}
	mockConfig["dogstatsd_socket"] = socketPath
	mockConfig["dogstatsd_origin_detection"] = false

	var contents = []byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2")

	packetsChannel := make(chan packets.Packets)

	config := fulfillDepsWithConfig(t, mockConfig)
	s, err := NewUDSListener(packetsChannel, newPacketPoolManagerUDS(config), config, nil)
	assert.Nil(t, err)
	assert.NotNil(t, s)

	go s.Listen()
	defer s.Stop()
	conn, err := net.Dial("unixgram", socketPath)
	assert.Nil(t, err)
	defer conn.Close()
	conn.Write(contents)

	select {
	case pkts := <-packetsChannel:
		packet := pkts[0]
		assert.NotNil(t, packet)
		assert.Equal(t, 1, len(pkts))
		assert.Equal(t, packet.Contents, contents)
		assert.Equal(t, packet.Origin, "")
		assert.Equal(t, packet.Source, packets.UDS)
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

}
