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

func TestUDSCustomReceiver(t *testing.T) {
	dir := t.TempDir()
	customSocket := filepath.Join(dir, "dsd_custom.socket") // custom socket

	cfg := make(map[string]interface{})
	cfg["dogstatsd_port"] = listeners.RandomPortName
	cfg["dogstatsd_no_aggregation_pipeline"] = true // another test may have turned it off
	cfg["dogstatsd_socket"] = customSocket

	deps := fulfillDepsWithConfigOverride(t, cfg)
	demux := deps.Demultiplexer

	conn, err := net.Dial("unixgram", customSocket)
	require.NoError(t, err, "cannot connect to DSD socket")
	defer conn.Close()

	testReceive(t, conn, demux)
}

func TestUDSDefaultReceiver(t *testing.T) {
	cfg := make(map[string]interface{})
	cfg["dogstatsd_port"] = listeners.RandomPortName
	cfg["dogstatsd_no_aggregation_pipeline"] = true // another test may have turned it off

	socket := defaultSocket
	defer func() {
		defaultSocket = socket
	}()

	dir := t.TempDir()
	defaultSocket = filepath.Join(dir, "dsd.socket") // default socket

	deps := fulfillDepsWithConfigOverride(t, cfg)
	demux := deps.Demultiplexer

	conn, err := net.Dial("unixgram", defaultSocket)
	require.NoError(t, err, "cannot connect to DSD socket")
	defer conn.Close()

	testReceive(t, conn, demux)
}
