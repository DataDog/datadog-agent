// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package traps

import (
	"fmt"
	"net"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/gosnmp/gosnmp"
)

// SnmpPacket is the type of packets yielded by server listeners.
type SnmpPacket struct {
	Content   *gosnmp.SnmpPacket
	Addr      *net.UDPAddr
	Timestamp int64
}

// PacketsChannel is the type of channels of trap packets.
type PacketsChannel = chan *SnmpPacket

// TrapServer manages an SNMP trap listener.
type TrapServer struct {
	Addr     string
	config   *Config
	listener *TrapListener
	sender   *TrapForwarder
}

var (
	serverInstance *TrapServer
	startError     error
)

// StartServer starts the global trap server.
func StartServer(agentHostname string, demux aggregator.Demultiplexer) error {
	sender, err := demux.GetDefaultSender()
	if err != nil {
		return err
	}
	oidResolver, err := NewMultiFilesOIDResolver()
	if err != nil {
		return err
	}
	formatter, err := NewJSONFormatter(oidResolver)
	if err != nil {
		return err
	}
	server, err := NewTrapServer(agentHostname, formatter, sender)
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

// GetNamespace returns the device namespace for the traps listener.
func GetNamespace() string {
	if serverInstance != nil {
		return serverInstance.config.Namespace
	}
	return defaultNamespace
}

// NewTrapServer configures and returns a running SNMP traps server.
func NewTrapServer(agentHostname string, formatter Formatter, aggregator aggregator.Sender) (*TrapServer, error) {
	config, err := ReadConfig(agentHostname)
	if err != nil {
		return nil, err
	}

	packets := make(PacketsChannel, packetsChanSize)

	listener, err := startSNMPTrapListener(*config, packets)
	if err != nil {
		return nil, err
	}

	trapForwarder, err := startSNMPTrapForwarder(formatter, aggregator, packets)
	if err != nil {
		return nil, fmt.Errorf("unable to start trapForwarder: %w. Will not listen for SNMP traps", err)
	}
	server := &TrapServer{
		listener: listener,
		config:   config,
		sender:   trapForwarder,
	}

	return server, nil
}

func startSNMPTrapForwarder(formatter Formatter, aggregator aggregator.Sender, packets PacketsChannel) (*TrapForwarder, error) {
	trapForwarder, err := NewTrapForwarder(formatter, aggregator, packets)
	if err != nil {
		return nil, err
	}
	trapForwarder.Start()
	return trapForwarder, nil
}
func startSNMPTrapListener(c Config, packets PacketsChannel) (*TrapListener, error) {
	trapListener, err := NewTrapListener(c, packets)
	if err != nil {
		return nil, err
	}
	err = trapListener.Start()
	if err != nil {
		return nil, err
	}
	return trapListener, nil
}

// Stop stops the TrapServer.
func (s *TrapServer) Stop() {
	stopped := make(chan interface{})

	go func() {
		log.Infof("Stop listening on %s", s.config.Addr())
		s.listener.Stop()
		s.sender.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(time.Duration(s.config.StopTimeout) * time.Second):
		log.Errorf("Stopping server. Timeout after %d seconds", s.config.StopTimeout)
	}
}
