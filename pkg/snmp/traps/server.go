// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package traps

import (
	"net"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/gosnmp/gosnmp"
)

// SnmpPacket is the type of packets yielded by server listeners.
type SnmpPacket struct {
	Content *gosnmp.SnmpPacket
	Addr    *net.UDPAddr
}

// PacketsChannel is the type of channels of trap packets.
type PacketsChannel = chan *SnmpPacket

// TrapServer manages an SNMPv2 trap listener.
type TrapServer struct {
	Addr     string
	config   *Config
	listener *gosnmp.TrapListener
	packets  PacketsChannel
}

var (
	serverInstance *TrapServer
	startError     error
)

// StartServer starts the global trap server.
func StartServer() error {
	server, err := NewTrapServer()
	serverInstance = server
	startError = err
	return err
}

// StopServer stops the global trap server, if it is running.
func StopServer() {
	if serverInstance != nil {
		serverInstance.Stop()
		serverInstance = nil
		startError = nil
	}
}

// IsRunning returns whether the trap server is currently running.
func IsRunning() bool {
	return serverInstance != nil
}

// GetPacketsChannel returns a channel containing all received trap packets.
func GetPacketsChannel() PacketsChannel {
	return serverInstance.packets
}

// NewTrapServer configures and returns a running SNMP traps server.
func NewTrapServer() (*TrapServer, error) {
	config, err := ReadConfig()
	if err != nil {
		return nil, err
	}

	packets := make(PacketsChannel, packetsChanSize)

	listener, err := startSNMPv2Listener(config, packets)
	if err != nil {
		return nil, err
	}

	server := &TrapServer{
		listener: listener,
		config:   config,
		packets:  packets,
	}

	return server, nil
}

func startSNMPv2Listener(c *Config, packets PacketsChannel) (*gosnmp.TrapListener, error) {
	listener := gosnmp.NewTrapListener()
	listener.Params = c.BuildV2Params()

	listener.OnNewTrap = func(p *gosnmp.SnmpPacket, u *net.UDPAddr) {
		if err := validateCredentials(p, c); err != nil {
			log.Warnf("Invalid credentials from %s on listener %s, dropping packet", u.String(), c.Addr())
			trapsPacketsAuthErrors.Add(1)
			return
		}
		log.Debugf("Packet received from %s on listener %s", u.String(), c.Addr())
		trapsPackets.Add(1)
		packets <- &SnmpPacket{Content: p, Addr: u}
	}

	errors := make(chan error, 1)

	// Start actually listening in the background.
	go func() {
		log.Infof("Start listening for traps on %s", c.Addr())
		err := listener.Listen(c.Addr())
		if err != nil {
			errors <- err
		}
	}()

	select {
	// Wait for listener to be started and listening to traps.
	// See: https://godoc.org/github.com/gosnmp/gosnmp#TrapListener.Listening
	case <-listener.Listening():
		break
	// If the listener failed to start (eg because it couldn't bind to a socket),
	// we'll get an error here.
	case err := <-errors:
		close(errors)
		return nil, err
	}

	return listener, nil
}

// Stop stops the TrapServer.
func (s *TrapServer) Stop() {
	stopped := make(chan interface{})

	go func() {
		log.Infof("Stop listening on %s", s.config.Addr())
		s.listener.Close()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(time.Duration(s.config.StopTimeout) * time.Second):
		log.Errorf("Stopping server. Timeout after %d seconds", s.config.StopTimeout)
	}

	// Let consumers know that we will not be sending any more packets.
	close(s.packets)
}
