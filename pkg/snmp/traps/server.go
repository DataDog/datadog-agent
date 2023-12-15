// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

// Package traps implements a server that listens for SNMP traps and forwards
// useful information to the DD backend.
package traps

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	trapsconfig "github.com/DataDog/datadog-agent/pkg/snmp/traps/config"
	"github.com/DataDog/datadog-agent/pkg/snmp/traps/formatter"
	"github.com/DataDog/datadog-agent/pkg/snmp/traps/forwarder"
	"github.com/DataDog/datadog-agent/pkg/snmp/traps/listener"
	oidresolver "github.com/DataDog/datadog-agent/pkg/snmp/traps/oid_resolver"
	"github.com/DataDog/datadog-agent/pkg/snmp/traps/packet"
	"github.com/DataDog/datadog-agent/pkg/snmp/traps/status"
)

// TrapServer manages an SNMP trap listener.
type TrapServer struct {
	Addr     string
	config   *trapsconfig.TrapsConfig
	listener *listener.TrapListener
	sender   *forwarder.TrapForwarder
	logger   log.Component
}

var (
	serverInstance *TrapServer
)

// StartServer starts the global trap server.
func StartServer(agentHostname string, demux aggregator.Demultiplexer, conf config.Component, logger log.Component) error {
	config, err := trapsconfig.ReadConfig(agentHostname, conf)
	if err != nil {
		return err
	}
	sender, err := demux.GetDefaultSender()
	if err != nil {
		return err
	}
	oidResolver, err := oidresolver.NewMultiFilesOIDResolver(conf.GetString("confd_path"), logger)
	if err != nil {
		return err
	}
	formatter, err := formatter.NewJSONFormatter(oidResolver, sender, logger)
	if err != nil {
		return err
	}
	status := status.New()
	server, err := NewTrapServer(config, formatter, sender, logger, status)
	serverInstance = server
	return err
}

// StopServer stops the global trap server, if it is running.
func StopServer() {
	if serverInstance != nil {
		serverInstance.Stop()
		serverInstance = nil
	}
}

// IsRunning returns whether the trap server is currently running.
func IsRunning() bool {
	return serverInstance != nil
}

// NewTrapServer configures and returns a running SNMP traps server.
func NewTrapServer(config *trapsconfig.TrapsConfig, formatter formatter.Formatter, aggregator sender.Sender, logger log.Component, status status.Manager) (*TrapServer, error) {
	packets := make(packet.PacketsChannel, config.GetPacketChannelSize())

	listener, err := startSNMPTrapListener(config, aggregator, packets, logger, status)
	if err != nil {
		return nil, err
	}

	trapForwarder, err := startSNMPTrapForwarder(formatter, aggregator, packets, logger)
	if err != nil {
		return nil, fmt.Errorf("unable to start trapForwarder: %w. Will not listen for SNMP traps", err)
	}
	server := &TrapServer{
		listener: listener,
		config:   config,
		sender:   trapForwarder,
		logger:   logger,
	}

	return server, nil
}

func startSNMPTrapForwarder(formatter formatter.Formatter, aggregator sender.Sender, packets packet.PacketsChannel, logger log.Component) (*forwarder.TrapForwarder, error) {
	trapForwarder, err := forwarder.NewTrapForwarder(formatter, aggregator, packets, logger)
	if err != nil {
		return nil, err
	}
	trapForwarder.Start()
	return trapForwarder, nil
}
func startSNMPTrapListener(c *trapsconfig.TrapsConfig, aggregator sender.Sender, packets packet.PacketsChannel, logger log.Component, status status.Manager) (*listener.TrapListener, error) {
	trapListener, err := listener.NewTrapListener(c, aggregator, packets, logger, status)
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
		s.logger.Infof("Stop listening on %s", s.config.Addr())
		s.listener.Stop()
		s.sender.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(time.Duration(s.config.StopTimeout) * time.Second):
		s.logger.Errorf("Stopping server. Timeout after %d seconds", s.config.StopTimeout)
	}
}

// IsEnabled returns whether SNMP trap collection is enabled in the Agent configuration.
func IsEnabled(conf config.Component) bool {
	return conf.GetBool("network_devices.snmp_traps.enabled")
}

// GetStatus returns key-value data for use in status reporting of the traps server.
func GetStatus() map[string]interface{} {
	return status.GetStatus()
}
