// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020 Datadog, Inc.

package traps

import (
	"net"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/soniah/gosnmp"
)

/*
* Traps pipeline diagram
* ----------------------
*
* (devices) --[udp:port1]-!-> (Listener 1) \
*                         !                 (TrapServer) --[chan]--> (Forwarder) --[chan]--> (Rest of the logs pipeline...)
* (devices) --[udp:portN]-!-> (Listener N) /
*
* ! = delimitation between the outside world and the Agent
 */

// SnmpPacket is the type of packets yielded by server listeners.
type SnmpPacket struct {
	Content *gosnmp.SnmpPacket
	Addr    *net.UDPAddr
}

// PacketsChannel is the type of the server output channel.
type PacketsChannel = chan *SnmpPacket

// TrapServer manages an SNMPv2 trap listener.
type TrapServer struct {
	Addr     string
	config   *Config
	listener *gosnmp.TrapListener
	packets  PacketsChannel
}

var (
	outputChannelSize = 100
	server            *TrapServer
	startError        error
)

// StartServer starts the global trap server.
func StartServer() error {
	s, err := NewTrapServer()
	server = s
	startError = err
	return err
}

// StopServer stops the global trap server, if it is running.
func StopServer() {
	if server != nil {
		server.Stop()
		server = nil
		startError = nil
	}
}

// IsRunning returns whether the trap server is currently running.
func IsRunning() bool {
	return server != nil
}

// GetPacketsChannel returns a channel containing all received trap packets.
func GetPacketsChannel() PacketsChannel {
	return server.packets
}

// NewTrapServer configures and returns a running SNMP traps server.
func NewTrapServer() (*TrapServer, error) {
	c, err := ReadConfig()
	if err != nil {
		return nil, err
	}

	packets := make(PacketsChannel, outputChannelSize)

	listener, err := startSNMPv2Listener(c, packets)
	if err != nil {
		return nil, err
	}

	s := &TrapServer{
		listener: listener,
		config:   c,
		packets:  packets,
	}

	return s, nil
}

func startSNMPv2Listener(c *Config, packets PacketsChannel) (*gosnmp.TrapListener, error) {
	ln := gosnmp.NewTrapListener()
	ln.Params = c.BuildV2Params()

	ln.OnNewTrap = func(p *gosnmp.SnmpPacket, u *net.UDPAddr) {
		if !validateCredentials(p, c) {
			log.Warnf("Invalid credentials from %s on listener %s, dropping packet", u.String(), c.Addr())
			trapsPacketsAuthErrors.Add(1)
			return
		}
		log.Debugf("New valid packet received from %s on listener %s", u.String(), c.Addr())
		trapsPackets.Add(1)
		packets <- &SnmpPacket{Content: p, Addr: u}
	}

	// Listening occurs in the background.
	// It can only terminate if an error occurs (1), or the listener is closed.
	// We wait on a channel (2) provided by GoSNMP to detect when the listener is ready to receive traps.

	errors := make(chan error, 1)

	go func() {
		log.Infof("Start listening for traps on %s", c.Addr())
		err := ln.Listen(c.Addr()) // (1)
		if err != nil {
			errors <- err
		}
	}()

	select {
	case <-ln.Listening(): // (2)
		break
	case err := <-errors:
		close(errors)
		return nil, err
	}

	return ln, nil
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
	case <-time.After(s.config.StopTimeout):
		log.Error("Stopping server timed out")
	}

	// Let consumers know that we will not be sending any more packets.
	close(s.packets)
}
