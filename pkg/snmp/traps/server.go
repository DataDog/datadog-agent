// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package traps

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TrapServer manages an SNMP trap listener.
type TrapServer struct {
	Addr     string
	config   Config
	listener *TrapListener
	sender   *TrapForwarder
}

var (
	serverInstance *TrapServer
	startError     error
)

// StartServer starts the global trap server.
func StartServer(agentHostname string, demux aggregator.Demultiplexer, conf config.Component) error {
	config, err := ReadConfig(agentHostname, conf)
	if err != nil {
		return err
	}
	sender, err := demux.GetDefaultSender()
	if err != nil {
		return err
	}
	oidResolver, err := NewMultiFilesOIDResolver(conf.GetString("confd_path"))
	if err != nil {
		return err
	}
	formatter, err := NewJSONFormatter(oidResolver, sender)
	if err != nil {
		return err
	}
	server, err := NewTrapServer(*config, formatter, sender)
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

// NewTrapServer configures and returns a running SNMP traps server.
func NewTrapServer(config Config, formatter Formatter, aggregator sender.Sender) (*TrapServer, error) {
	packets := make(PacketsChannel, packetsChanSize)

	listener, err := startSNMPTrapListener(config, aggregator, packets)
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

func startSNMPTrapForwarder(formatter Formatter, aggregator sender.Sender, packets PacketsChannel) (*TrapForwarder, error) {
	trapForwarder, err := NewTrapForwarder(formatter, aggregator, packets)
	if err != nil {
		return nil, err
	}
	trapForwarder.Start()
	return trapForwarder, nil
}
func startSNMPTrapListener(c Config, aggregator sender.Sender, packets PacketsChannel) (*TrapListener, error) {
	trapListener, err := NewTrapListener(c, aggregator, packets)
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
