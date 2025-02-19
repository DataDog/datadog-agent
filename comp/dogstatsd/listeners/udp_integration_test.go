// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build !windows

package listeners

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
)

func TestStartStopUDPListener(t *testing.T) {
	port, err := getAvailableUDPPort()
	require.NoError(t, err)
	cfg := map[string]interface{}{}
	cfg["dogstatsd_port"] = port
	cfg["dogstatsd_non_local_traffic"] = false

	deps := fulfillDepsWithConfig(t, cfg)
	telemetryStore := NewTelemetryStore(nil, deps.Telemetry)
	packetsTelemetryStore := packets.NewTelemetryStore(nil, deps.Telemetry)
	s, err := NewUDPListener(nil, newPacketPoolManagerUDP(deps.Config, packetsTelemetryStore), deps.Config, nil, telemetryStore, packetsTelemetryStore)
	require.NotNil(t, s)

	assert.NoError(t, err)

	s.Listen()
	// Local port should be unavailable
	address, _ := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", port))
	_, err = net.ListenUDP("udp", address)
	assert.Error(t, err)

	s.Stop()

	// check that the port can be bound, try for 100 ms
	for i := 0; i < 10; i++ {
		var conn net.Conn
		conn, err = net.ListenUDP("udp", address)
		if err == nil {
			conn.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.NoError(t, err, "port is not available, it should be")
}

func TestUDPNonLocal(t *testing.T) {
	port, err := getAvailableUDPPort()
	require.NoError(t, err)

	cfg := map[string]interface{}{}
	cfg["dogstatsd_port"] = port
	cfg["dogstatsd_non_local_traffic"] = true
	deps := fulfillDepsWithConfig(t, cfg)
	telemetryStore := NewTelemetryStore(nil, deps.Telemetry)
	packetsTelemetryStore := packets.NewTelemetryStore(nil, deps.Telemetry)
	s, err := NewUDPListener(nil, newPacketPoolManagerUDP(deps.Config, packetsTelemetryStore), deps.Config, nil, telemetryStore, packetsTelemetryStore)
	assert.NoError(t, err)
	require.NotNil(t, s)

	s.Listen()
	defer s.Stop()

	// Local port should be unavailable
	address, _ := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", port))
	_, err = net.ListenUDP("udp", address)
	assert.Error(t, err)

	// External port should be unavailable
	externalPort := fmt.Sprintf("%s:%d", getLocalIP(), port)
	address, _ = net.ResolveUDPAddr("udp", externalPort)
	_, err = net.ListenUDP("udp", address)
	assert.Error(t, err)
}

func TestUDPLocalOnly(t *testing.T) {
	port, err := getAvailableUDPPort()
	require.NoError(t, err)

	fmt.Println("port: ", port)

	cfg := map[string]interface{}{}
	cfg["dogstatsd_port"] = port
	cfg["dogstatsd_non_local_traffic"] = false
	deps := fulfillDepsWithConfig(t, cfg)
	telemetryStore := NewTelemetryStore(nil, deps.Telemetry)
	packetsTelemetryStore := packets.NewTelemetryStore(nil, deps.Telemetry)
	s, err := NewUDPListener(nil, newPacketPoolManagerUDP(deps.Config, packetsTelemetryStore), deps.Config, nil, telemetryStore, packetsTelemetryStore)
	assert.NoError(t, err)
	require.NotNil(t, s)

	s.Listen()
	defer s.Stop()

	// Local port should be unavailable
	address, _ := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", port))
	_, err = net.ListenUDP("udp", address)
	assert.Error(t, err)

	// External port should be available
	externalPort := fmt.Sprintf("%s:%d", getLocalIP(), port)
	address, _ = net.ResolveUDPAddr("udp", externalPort)
	conn, err := net.ListenUDP("udp", address)
	require.NotNil(t, conn)
	assert.NoError(t, err)
	conn.Close()
}
