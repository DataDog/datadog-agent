// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package flow

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"time"

	"github.com/netsampler/goflow2/utils"
	logrus "github.com/sirupsen/logrus"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Server manages an SNMPv2 trap listener.
type Server struct {
	Addr          string
	config        *Config
	listener      *utils.StateNetFlow
	demultiplexer aggregator.Demultiplexer
}

var (
	serverInstance *Server
)

// StartServer starts the global trap server.
func StartServer(demultiplexer aggregator.Demultiplexer) error {
	server, err := NewNetflowServer(demultiplexer)
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

// NewNetflowServer configures and returns a running SNMP traps server.
func NewNetflowServer(demultiplexer aggregator.Demultiplexer) (*Server, error) {
	config, err := ReadConfig()
	if err != nil {
		return nil, err
	}

	listener, err := startSNMPv2Listener(config, demultiplexer)
	if err != nil {
		return nil, err
	}

	server := &Server{
		listener:      listener,
		config:        config,
		demultiplexer: demultiplexer,
	}

	return server, nil
}

func startSNMPv2Listener(c *Config, demultiplexer aggregator.Demultiplexer) (*utils.StateNetFlow, error) {
	log.Warn("Starting Netflow Server")
	//agg := demultiplexer.Aggregator()
	sender, err := demultiplexer.GetDefaultSender()
	if err != nil {
		return nil, err
	}
	ndmFlowDriver := NewFlowDriver(sender, c)

	logger := logrus.StandardLogger()
	logger.SetLevel(logrus.TraceLevel)
	sNF := &utils.StateNetFlow{
		Format: ndmFlowDriver,
		Logger: logger,
	}
	hostname := c.BindHost
	port := c.Port
	reusePort := false
	go func() {
		log.Errorf("Starting FlowRoutine...")
		err = sNF.FlowRoutine(1, hostname, int(port), reusePort)
		log.Errorf("Exited FlowRoutine")
		if err != nil {
			log.Errorf("Error exiting FlowRoutine: %s", err)
		}
	}()

	return sNF, nil
}

// Stop stops the Server.
func (s *Server) Stop() {
	log.Infof("Stop listening on %s", s.config.Addr())
	stopped := make(chan interface{})

	go func() {
		log.Infof("Stop listening on %s", s.config.Addr())
		s.listener.Shutdown()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(time.Duration(s.config.StopTimeout) * time.Second):
		log.Errorf("Stopping server. Timeout after %d seconds", s.config.StopTimeout)
	}
}
