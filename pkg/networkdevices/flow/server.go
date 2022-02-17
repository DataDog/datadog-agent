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
	listeners     []Listener
	demultiplexer aggregator.Demultiplexer
}

type Listener struct {
	flowState interface{}
	config    ListenerConfig
}

func (l Listener) Shutdown() {
	switch state := l.flowState.(type) {
	case utils.StateNetFlow:
		state.Shutdown()
	case utils.StateSFlow:
		state.Shutdown()
	case utils.StateNFLegacy:
		state.Shutdown()
	default:
		log.Warn("Unknown flow listener state (%v) for %s", state, l.config.Addr())
	}
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
	flowConfigs, err := ReadConfig()
	if err != nil {
		return nil, err
	}

	var listeners []Listener

	for _, config := range flowConfigs.configs {
		listener, err := startSNMPv2Listener(config, demultiplexer)
		if err != nil {
			log.Warn("Error starting listener for config (flow_type:%s, bind_Host:%s, port:%d)", config.FlowType, config.BindHost, config.Port)
		}
		listeners = append(listeners, Listener{
			flowState: listener,
			config:    config,
		})
	}

	server := &Server{
		listeners:     listeners,
		demultiplexer: demultiplexer,
	}

	return server, nil
}

func startSNMPv2Listener(listenerConfig ListenerConfig, demultiplexer aggregator.Demultiplexer) (*utils.StateNetFlow, error) {
	log.Warn("Starting Netflow Server")
	//agg := demultiplexer.Aggregator()
	sender, err := demultiplexer.GetDefaultSender()
	if err != nil {
		return nil, err
	}
	ndmFlowDriver := NewFlowDriver(sender, listenerConfig)

	logger := logrus.StandardLogger()
	logger.SetLevel(logrus.TraceLevel)
	sNF := &utils.StateNetFlow{
		Format: ndmFlowDriver,
		Logger: logger,
	}
	hostname := listenerConfig.BindHost
	port := listenerConfig.Port
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
	for _, listener := range s.listeners {
		// TODO: shutdown concurrently

		log.Infof("Stop listening on %s", listener.config.Addr())
		stopped := make(chan interface{})

		go func() {
			log.Infof("Stop listening on %s", listener.config.Addr())
			listener.Shutdown()
			close(stopped)
		}()

		select {
		case <-stopped:
		case <-time.After(time.Duration(s.config.StopTimeout) * time.Second):
			log.Errorf("Stopping server. Timeout after %d seconds", s.config.StopTimeout)
		}
	}
}
