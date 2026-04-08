// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listener

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/pipeline/mock"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// use a randomly assigned port
var udpTestPort = 0

// localhostDenyCIDRs covers both IPv4 and IPv6 loopback since the OS may
// deliver datagrams from either address family depending on the socket type.
var localhostDenyCIDRs = []string{"127.0.0.0/8", "::1/128"}
var localhostAllowCIDRs = []string{"127.0.0.0/8", "::1/128"}

func TestUDPShouldReceiveMessage(t *testing.T) {
	pp := mock.NewMockProvider()
	msgChan := pp.NextPipelineChan()
	listener, err := NewUDPListener(pp, sources.NewLogSource("", &config.LogsConfig{Port: udpTestPort}), 9000)
	require.NoError(t, err)
	listener.Start()

	conn, err := net.Dial("udp", listener.Conn.LocalAddr().String())
	assert.Nil(t, err)

	var msg *message.Message

	fmt.Fprint(conn, "hello world\n")
	msg = <-msgChan
	assert.Equal(t, "hello world", string(msg.GetContent()))

	listener.Stop()
}

func TestUDPShouldStopWhenNotStarted(t *testing.T) {
	pp := mock.NewMockProvider()
	listener, err := NewUDPListener(pp, sources.NewLogSource("", &config.LogsConfig{Port: udpTestPort}), 9000)
	require.NoError(t, err)
	listener.Stop()
}

func TestUDPAllowedIPsAcceptsMatchingDatagram(t *testing.T) {
	pp := mock.NewMockProvider()
	msgChan := pp.NextPipelineChan()
	listener, err := NewUDPListener(pp, sources.NewLogSource("", &config.LogsConfig{
		Port:       udpTestPort,
		AllowedIPs: localhostAllowCIDRs,
	}), 9000)
	require.NoError(t, err)
	listener.Start()

	conn, err := net.Dial("udp", listener.tailer.Conn.LocalAddr().String())
	require.NoError(t, err)

	fmt.Fprint(conn, "allowed\n")
	msg := <-msgChan
	assert.Equal(t, "allowed", string(msg.GetContent()))

	listener.Stop()
}

func TestUDPDeniedIPsDropsMatchingDatagram(t *testing.T) {
	pp := mock.NewMockProvider()
	msgChan := pp.NextPipelineChan()
	listener, err := NewUDPListener(pp, sources.NewLogSource("", &config.LogsConfig{
		Port:      udpTestPort,
		DeniedIPs: localhostDenyCIDRs,
	}), 9000)
	require.NoError(t, err)
	listener.Start()

	conn, err := net.Dial("udp", listener.tailer.Conn.LocalAddr().String())
	require.NoError(t, err)
	defer conn.Close()

	fmt.Fprint(conn, "should be dropped\n")

	select {
	case <-msgChan:
		t.Fatal("expected datagram from denied IP to be dropped")
	case <-time.After(200 * time.Millisecond):
	}

	listener.Stop()
}

func TestUDPDenyTakesPrecedenceOverAllow(t *testing.T) {
	pp := mock.NewMockProvider()
	msgChan := pp.NextPipelineChan()
	listener, err := NewUDPListener(pp, sources.NewLogSource("", &config.LogsConfig{
		Port:       udpTestPort,
		AllowedIPs: localhostAllowCIDRs,
		DeniedIPs:  localhostDenyCIDRs,
	}), 9000)
	require.NoError(t, err)
	listener.Start()

	conn, err := net.Dial("udp", listener.tailer.Conn.LocalAddr().String())
	require.NoError(t, err)
	defer conn.Close()

	fmt.Fprint(conn, "denied takes precedence\n")

	select {
	case <-msgChan:
		t.Fatal("expected datagram to be dropped (deny takes precedence over allow)")
	case <-time.After(200 * time.Millisecond):
	}

	listener.Stop()
}
