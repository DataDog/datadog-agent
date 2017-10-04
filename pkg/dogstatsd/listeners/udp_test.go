// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.
// +build !windows

package listeners

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestNewUDPListener(t *testing.T) {
	s, err := NewUDPListener(nil)
	defer s.Stop()

	assert.Nil(t, err)
	assert.NotNil(t, s)
}

func TestStartStopUDPListener(t *testing.T) {
	config.Datadog.Set("dogstatsd_non_local_traffic", false)
	s, err := NewUDPListener(nil)
	assert.Nil(t, err)
	assert.NotNil(t, s)

	go s.Listen()
	// Local port should be unavailable
	address, _ := net.ResolveUDPAddr("udp", "127.0.0.1:8125")
	_, err = net.ListenUDP("udp", address)
	assert.NotNil(t, err)

	s.Stop()
	// Port should be available again
	conn, err := net.ListenUDP("udp", address)
	assert.Nil(t, err)
	conn.Close()
}

func TestUDPNonLocal(t *testing.T) {
	config.Datadog.Set("dogstatsd_non_local_traffic", true)
	s, err := NewUDPListener(nil)
	go s.Listen()
	defer s.Stop()

	// Local port should be unavailable
	address, _ := net.ResolveUDPAddr("udp", "127.0.0.1:8125")
	_, err = net.ListenUDP("udp", address)
	assert.NotNil(t, err)

	// External port should be unavailable
	externalPort := fmt.Sprintf("%s:8125", getLocalIP())
	address, _ = net.ResolveUDPAddr("udp", externalPort)
	_, err = net.ListenUDP("udp", address)
	assert.NotNil(t, err)
}

func TestUDPLocalOnly(t *testing.T) {
	config.Datadog.Set("dogstatsd_non_local_traffic", false)
	s, err := NewUDPListener(nil)
	go s.Listen()
	defer s.Stop()

	// Local port should be unavailable
	address, _ := net.ResolveUDPAddr("udp", "127.0.0.1:8125")
	_, err = net.ListenUDP("udp", address)
	assert.NotNil(t, err)

	// External port should be available
	externalPort := fmt.Sprintf("%s:8125", getLocalIP())
	address, _ = net.ResolveUDPAddr("udp", externalPort)
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
		assert.Equal(t, contents, packet.Contents)
		assert.Equal(t, "", packet.Origin)
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}
}

// getLocalIP returns the first non loopback local IPv4 on that host
func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}
