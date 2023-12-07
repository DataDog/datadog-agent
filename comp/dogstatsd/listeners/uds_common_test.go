// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// UDS won't work in windows

package listeners

import (
	"encoding/binary"
	"net"
	"os"
	"testing"
	"time"

	"golang.org/x/net/nettest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
)

type udsListenerFactory func(packetOut chan packets.Packets, manager *packets.PoolManager, cfg config.Component) (StatsdListener, error)

func socketPathConfKey(transport string) string {
	if transport == "unix" {
		return "dogstatsd_stream_socket"
	}
	return "dogstatsd_socket"
}

func testSocketPath(t *testing.T) string {
	// https://github.com/golang/go/issues/62614
	path, err := nettest.LocalPath()
	assert.NoError(t, err)
	return path
}

func newPacketPoolManagerUDS(cfg config.Component) *packets.PoolManager {
	packetPoolUDS := packets.NewPool(cfg.GetInt("dogstatsd_buffer_size"))
	return packets.NewPoolManager(packetPoolUDS)
}

func testFileExistsNewUDSListener(t *testing.T, socketPath string, cfg map[string]interface{}, listenerFactory udsListenerFactory) {
	_, err := os.Create(socketPath)
	assert.Nil(t, err)
	defer os.Remove(socketPath)
	config := fulfillDepsWithConfig(t, cfg)
	_, err = listenerFactory(nil, newPacketPoolManagerUDS(config), config)
	assert.Error(t, err)
}

func testSocketExistsNewUSDListener(t *testing.T, socketPath string, cfg map[string]interface{}, listenerFactory udsListenerFactory) {
	address, err := net.ResolveUnixAddr("unix", socketPath)
	assert.Nil(t, err)
	_, err = net.ListenUnix("unix", address)
	assert.Nil(t, err)
	testWorkingNewUDSListener(t, socketPath, cfg, listenerFactory)
}

func testWorkingNewUDSListener(t *testing.T, socketPath string, cfg map[string]interface{}, listenerFactory udsListenerFactory) {
	config := fulfillDepsWithConfig(t, cfg)
	s, err := listenerFactory(nil, newPacketPoolManagerUDS(config), config)
	defer s.Stop()

	assert.Nil(t, err)
	assert.NotNil(t, s)
	fi, err := os.Stat(socketPath)
	require.Nil(t, err)
	assert.Equal(t, "Srwx-w--w-", fi.Mode().String())
}

func testNewUDSListener(t *testing.T, listenerFactory udsListenerFactory, transport string) {
	socketPath := testSocketPath(t)

	mockConfig := map[string]interface{}{}
	mockConfig[socketPathConfKey(transport)] = socketPath

	t.Run("fail_file_exists", func(tt *testing.T) {
		testFileExistsNewUDSListener(tt, socketPath, mockConfig, listenerFactory)
	})
	t.Run("socket_exists", func(tt *testing.T) {
		testSocketExistsNewUSDListener(tt, socketPath, mockConfig, listenerFactory)
	})
	t.Run("working", func(tt *testing.T) {
		testWorkingNewUDSListener(tt, socketPath, mockConfig, listenerFactory)
	})
}

func testStartStopUDSListener(t *testing.T, listenerFactory udsListenerFactory, transport string) {
	socketPath := testSocketPath(t)

	mockConfig := map[string]interface{}{}
	mockConfig[socketPathConfKey(transport)] = socketPath
	mockConfig["dogstatsd_origin_detection"] = false

	config := fulfillDepsWithConfig(t, mockConfig)
	s, err := listenerFactory(nil, newPacketPoolManagerUDS(config), config)
	assert.Nil(t, err)
	assert.NotNil(t, s)

	s.Listen()

	conn, err := net.Dial(transport, socketPath)
	assert.Nil(t, err)
	conn.Close()

	s.Stop()

	_, err = net.Dial(transport, socketPath)
	assert.NotNil(t, err)
}

func testUDSReceive(t *testing.T, listenerFactory udsListenerFactory, transport string) {
	socketPath := testSocketPath(t)

	mockConfig := map[string]interface{}{}
	mockConfig[socketPathConfKey(transport)] = socketPath
	mockConfig["dogstatsd_origin_detection"] = false

	var contents0 = []byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2")
	var contents1 = []byte("daemon:999|g|#sometag1:somevalue1")

	packetsChannel := make(chan packets.Packets)

	config := fulfillDepsWithConfig(t, mockConfig)
	s, err := listenerFactory(packetsChannel, newPacketPoolManagerUDS(config), config)
	assert.Nil(t, err)
	assert.NotNil(t, s)

	s.Listen()
	defer s.Stop()
	conn, err := net.Dial(transport, socketPath)
	assert.Nil(t, err)
	defer conn.Close()

	if transport == "unix" {
		binary.Write(conn, binary.LittleEndian, int32(len(contents0)))
	}
	conn.Write(contents0)

	if transport == "unix" {
		binary.Write(conn, binary.LittleEndian, int32(len(contents1)))
	}
	conn.Write(contents1)

	select {
	case pkts := <-packetsChannel:
		assert.Equal(t, 2, len(pkts))

		packet := pkts[0]
		assert.NotNil(t, packet)
		assert.Equal(t, packet.Contents, contents0)
		assert.Equal(t, packet.Origin, "")
		assert.Equal(t, packet.Source, packets.UDS)

		packet = pkts[1]
		assert.NotNil(t, packet)
		assert.Equal(t, packet.Contents, contents1)
		assert.Equal(t, packet.Origin, "")
		assert.Equal(t, packet.Source, packets.UDS)

	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

}
