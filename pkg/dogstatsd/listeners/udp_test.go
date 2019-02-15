// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build !windows

package listeners

import (
	"fmt"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
)

var packetPoolUDP = NewPacketPool(config.Datadog.GetInt("dogstatsd_buffer_size"))

func TestNewUDPListener(t *testing.T) {
	s, err := NewUDPListener(nil, packetPoolUDP)
	require.NotNil(t, s)
	assert.Nil(t, err)

	s.Stop()
}

func TestStartStopUDPListener(t *testing.T) {
	port, err := getAvailableUDPPort()
	require.Nil(t, err)
	config.Datadog.SetDefault("dogstatsd_port", port)
	config.Datadog.SetDefault("dogstatsd_non_local_traffic", false)
	s, err := NewUDPListener(nil, packetPoolUDP)
	require.NotNil(t, s)

	assert.Nil(t, err)

	go s.Listen()
	// Local port should be unavailable
	address, _ := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", port))
	_, err = net.ListenUDP("udp", address)
	assert.NotNil(t, err)

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
	require.Nil(t, err)
	config.Datadog.SetDefault("dogstatsd_port", port)
	config.Datadog.SetDefault("dogstatsd_non_local_traffic", true)
	s, err := NewUDPListener(nil, packetPoolUDP)
	assert.Nil(t, err)
	require.NotNil(t, s)

	go s.Listen()
	defer s.Stop()

	// Local port should be unavailable
	address, _ := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", port))
	_, err = net.ListenUDP("udp", address)
	assert.NotNil(t, err)

	// External port should be unavailable
	externalPort := fmt.Sprintf("%s:%d", getLocalIP(), port)
	address, _ = net.ResolveUDPAddr("udp", externalPort)
	_, err = net.ListenUDP("udp", address)
	assert.NotNil(t, err)
}

func TestUDPLocalOnly(t *testing.T) {
	port, err := getAvailableUDPPort()
	require.Nil(t, err)
	config.Datadog.SetDefault("dogstatsd_port", port)
	config.Datadog.SetDefault("dogstatsd_non_local_traffic", false)
	s, err := NewUDPListener(nil, packetPoolUDP)
	assert.Nil(t, err)
	require.NotNil(t, s)

	go s.Listen()
	defer s.Stop()

	// Local port should be unavailable
	address, _ := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", port))
	_, err = net.ListenUDP("udp", address)
	assert.NotNil(t, err)

	// External port should be available
	externalPort := fmt.Sprintf("%s:%d", getLocalIP(), port)
	address, _ = net.ResolveUDPAddr("udp", externalPort)
	conn, err := net.ListenUDP("udp", address)
	require.NotNil(t, conn)
	assert.Nil(t, err)
	conn.Close()
}

func TestUDPReceive(t *testing.T) {
	var contents = []byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2")

	port, err := getAvailableUDPPort()
	require.Nil(t, err)
	config.Datadog.SetDefault("dogstatsd_port", port)

	packetChannel := make(chan Packets)
	s, err := NewUDPListener(packetChannel, packetPoolUDP)
	require.NotNil(t, s)
	assert.Nil(t, err)

	go s.Listen()
	defer s.Stop()
	conn, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", port))
	require.NotNil(t, conn)
	assert.Nil(t, err)
	defer conn.Close()
	conn.Write(contents)

	select {
	case packets := <-packetChannel:
		packet := packets[0]
		assert.NotNil(t, packet)
		assert.Equal(t, 1, len(packets))
		assert.Equal(t, contents, packet.Contents)
		assert.Equal(t, "", packet.Origin)
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}
}

// getAvailableUDPPort requests a random port number and makes sure it is available
func getAvailableUDPPort() (int, error) {
	conn, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return -1, fmt.Errorf("can't find an available udp port: %s", err)
	}
	defer conn.Close()

	_, portString, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		return -1, fmt.Errorf("can't find an available udp port: %s", err)
	}
	portInt, err := strconv.Atoi(portString)
	if err != nil {
		return -1, fmt.Errorf("can't convert udp port: %s", err)
	}

	return portInt, nil
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
