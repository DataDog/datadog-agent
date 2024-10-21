// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package server

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/listeners"
)

func TestUDPForward(t *testing.T) {
	cfg := make(map[string]interface{})

	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)

	pcHost, pcPort, err := net.SplitHostPort(pc.LocalAddr().String())
	require.NoError(t, err)

	// Setup UDP server to forward to
	cfg["statsd_forward_port"] = pcPort
	cfg["statsd_forward_host"] = pcHost

	// Setup dogstatsd server
	cfg["dogstatsd_port"] = listeners.RandomPortName

	deps := fulfillDepsWithConfigOverride(t, cfg)

	defer pc.Close()

	requireStart(t, deps.Server)

	conn, err := net.Dial("udp", deps.Server.UDPLocalAddr())
	require.NoError(t, err)
	require.NotNil(t, conn)
	defer conn.Close()

	// Check if message is forwarded
	_, err = conn.Write(defaultMetricInput)
	require.NoError(t, err, "cannot write to DSD socket")

	_ = pc.SetReadDeadline(time.Now().Add(4 * time.Second))

	buffer := make([]byte, len(defaultMetricInput))
	_, _, err = pc.ReadFrom(buffer)
	require.NoError(t, err)

	assert.Equal(t, defaultMetricInput, buffer)
}

func TestUDPConn(t *testing.T) {
	cfg := make(map[string]interface{})

	cfg["dogstatsd_port"] = listeners.RandomPortName

	deps := fulfillDepsWithConfigOverride(t, cfg)
	s := deps.Server.(*server)
	requireStart(t, s)

	conn, err := net.Dial("udp", s.UDPLocalAddr())
	require.NoError(t, err, "cannot connect to DSD socket")
	defer conn.Close()

	runConnTest(t, conn, deps)

	s.stop(context.TODO())

	// check that the port can be bound, try for 100 ms
	address, err := net.ResolveUDPAddr("udp", s.UDPLocalAddr())
	require.NoError(t, err, "cannot resolve address")

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

func runConnTest(t *testing.T, conn net.Conn, deps serverDeps) {
	demux := deps.Demultiplexer
	eventOut, serviceOut := demux.GetEventsAndServiceChecksChannels()

	// Test metric
	conn.Write(defaultMetricInput)
	samples, timedSamples := demux.WaitForSamples(time.Second * 2)

	assert.Equal(t, 1, len(samples), "expected one metric entries after 2 seconds")
	assert.Equal(t, 0, len(timedSamples), "did not expect any timed metrics")

	defaultMetric().testMetric(t, samples[0])

	// Test servce checks
	conn.Write(defaultServiceInput)
	select {
	case servL := <-serviceOut:
		assert.Equal(t, 1, len(servL))
		defaultServiceCheck().testService(t, servL[0])
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on service channel")
	}

	// Test event
	conn.Write(defaultEventInput)
	select {
	case eventL := <-eventOut:
		assert.Equal(t, 1, len(eventL))
		defaultEvent().testEvent(t, eventL[0])
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on event channel")
	}
}

func TestUDSConn(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "dsd.socket")

	cfg := make(map[string]interface{})
	cfg["dogstatsd_port"] = listeners.RandomPortName
	cfg["dogstatsd_no_aggregation_pipeline"] = true // another test may have turned it off
	cfg["dogstatsd_socket"] = socketPath

	deps := fulfillDepsWithConfigOverride(t, cfg)
	require.True(t, deps.Server.UdsListenerRunning())

	conn, err := net.Dial("unixgram", socketPath)
	require.NoError(t, err, "cannot connect to DSD socket")
	defer conn.Close()

	runConnTest(t, conn, deps)

	s := deps.Server.(*server)
	s.Stop()
	_, err = net.Dial("unixgram", socketPath)
	require.Error(t, err, "UDS listener should be closed")
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
