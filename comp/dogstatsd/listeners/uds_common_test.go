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
	"testing"

	"golang.org/x/net/nettest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap"
)

type udsListenerFactory func(packetOut chan packets.Packets, manager *packets.PoolManager, cfg config.Component, pidMap pidmap.Component) (StatsdListener, error)

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
	deps := fulfillDepsWithConfig(t, cfg)
	_, err = listenerFactory(nil, newPacketPoolManagerUDS(deps.Config), deps.Config, deps.PidMap)
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
	deps := fulfillDepsWithConfig(t, cfg)
	s, err := listenerFactory(nil, newPacketPoolManagerUDS(deps.Config), deps.Config, deps.PidMap)
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

	deps := fulfillDepsWithConfig(t, mockConfig)
	s, err := listenerFactory(nil, newPacketPoolManagerUDS(deps.Config), deps.Config, deps.PidMap)
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
