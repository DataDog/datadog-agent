// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build !windows
// UDS won't work in windows

package listeners

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewUDPListener(t *testing.T) {
	s, err := NewUDPListener(nil)
	defer s.Stop()

	assert.Nil(t, err)
	assert.NotNil(t, s)
}

func TestStartStopUDPListener(t *testing.T) {
	s, err := NewUDPListener(nil)
	assert.Nil(t, err)
	assert.NotNil(t, s)

	go s.Listen()
	// Port should be unavailable
	address, _ := net.ResolveUDPAddr("udp", "127.0.0.1:8125")
	_, err = net.ListenUDP("udp", address)
	assert.NotNil(t, err)

	s.Stop()
	// Port should be available again
	conn, err := net.ListenUDP("udp", address)
	assert.Nil(t, err)
	conn.Close()
}

func TestUDPReceive(t *testing.T) {
	var contents = []byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2")

	packetChannel := make(chan *Packet)
	s, err := NewUDPListener(packetChannel)
	assert.Nil(t, err)
	assert.NotNil(t, s)

	go s.Listen()
	defer s.Stop()
	conn, err := net.Dial("udp", "127.0.0.1:8125")
	assert.Nil(t, err)
	defer conn.Close()
	conn.Write(contents)

	select {
	case packet := <-packetChannel:
		assert.NotNil(t, packet)
		assert.Equal(t, packet.Contents, contents)
		assert.Equal(t, packet.Container, "")
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}
}
