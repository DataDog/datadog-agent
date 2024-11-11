// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// UDS won't work in windows

package server

import (
	"net"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/listeners"
)

func TestUDSReceiver(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "dsd.socket")

	cfg := make(map[string]interface{})
	cfg["dogstatsd_port"] = listeners.RandomPortName
	cfg["dogstatsd_no_aggregation_pipeline"] = true // another test may have turned it off
	cfg["dogstatsd_socket"] = socketPath

	deps := fulfillDepsWithConfigOverride(t, cfg)
	demux := deps.Demultiplexer
	require.True(t, deps.Server.UdsListenerRunning())

	conn, err := net.Dial("unixgram", socketPath)
	require.NoError(t, err, "cannot connect to DSD socket")
	defer conn.Close()

	testReceive(t, conn, demux)

	s := deps.Server.(*server)
	s.Stop()
	_, err = net.Dial("unixgram", socketPath)
	require.Error(t, err, "UDS listener should be closed")
}

func TestUDSReceiverDisabled(t *testing.T) {
	cfg := make(map[string]interface{})
	cfg["dogstatsd_port"] = listeners.RandomPortName
	cfg["dogstatsd_no_aggregation_pipeline"] = true // another test may have turned it off
	cfg["dogstatsd_socket"] = ""                    // disabled

	deps := fulfillDepsWithConfigOverride(t, cfg)
	require.False(t, deps.Server.UdsListenerRunning())
}

func TestUDSReceiverNoDir(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "nonexistent", "dsd.socket") // nonexistent dir, listener should not be set

	cfg := make(map[string]interface{})
	cfg["dogstatsd_port"] = listeners.RandomPortName
	cfg["dogstatsd_no_aggregation_pipeline"] = true // another test may have turned it off
	cfg["dogstatsd_socket"] = socketPath

	deps := fulfillDepsWithConfigOverride(t, cfg)
	require.False(t, deps.Server.UdsListenerRunning())

	_, err := net.Dial("unixgram", socketPath)
	require.Error(t, err, "UDS listener should be closed")
}
