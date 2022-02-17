// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package flow

import (
	"fmt"
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
	case *utils.StateNetFlow:
		log.Infof("Shutdown NetFlow9/IPFIX listener on %s", l.config.Addr())
		state.Shutdown()
	case *utils.StateSFlow:
		log.Infof("Shutdown sFlow listener on %s", l.config.Addr())
		state.Shutdown()
	case *utils.StateNFLegacy:
		log.Infof("Shutdown Netflow5 listener on %s", l.config.Addr())
		state.Shutdown()
	default:
		log.Warnf("Unknown flow listener state type `%T` for %s", state, l.config.Addr())
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
	allConfigs, err := ReadConfig()
	if err != nil {
		return nil, err
	}

	var listeners []Listener

	for _, config := range allConfigs.Configs {
		log.Infof("Starting Netflow listener for flow type %s on %s", config.FlowType, config.Addr())
		listener, err := startFlowListeners(config, demultiplexer)
		if err != nil {
			log.Warn("Error starting listener for config (flow_type:%s, bind_Host:%s, port:%d)", config.FlowType, config.BindHost, config.Port)
		} else {
			listeners = append(listeners, listener)
		}
	}

	server := &Server{
		listeners:     listeners,
		demultiplexer: demultiplexer,
		config:        allConfigs,
	}

	return server, nil
}

func startFlowListeners(listenerConfig ListenerConfig, demultiplexer aggregator.Demultiplexer) (Listener, error) {
	//agg := demultiplexer.Aggregator()
	sender, err := demultiplexer.GetDefaultSender()
	if err != nil {
		return Listener{}, err
	}
	ndmFlowDriver := newSenderDriver(sender, listenerConfig)

	// TODO: Match logger with agent logger
	logger := logrus.StandardLogger()
	logger.SetLevel(logrus.InfoLevel)

	hostname := listenerConfig.BindHost
	port := listenerConfig.Port
	reusePort := false

	var flowState interface{}
	switch listenerConfig.FlowType {
	case NETFLOW9, IPFIX:
		log.Info("Starting NetFlow9/IPFIX listener...")
		stateNetFlow := &utils.StateNetFlow{
			Format: ndmFlowDriver,
			Logger: logger,
		}
		flowState = stateNetFlow

		go func() {
			err = stateNetFlow.FlowRoutine(1, hostname, int(port), reusePort)
			if err != nil {
				log.Errorf("Error listener to netflow9/ipfix: %s", err)
			}
		}()
	case SFLOW:
		log.Info("Starting sFlow listener ...")
		stateSFlow := &utils.StateSFlow{
			Format: ndmFlowDriver,
			Logger: logger,
		}
		flowState = stateSFlow
		go func() {
			err = stateSFlow.FlowRoutine(1, hostname, int(port), reusePort)
			if err != nil {
				log.Errorf("Error listener to sflow: %s", err)
			}
		}()
	case NETFLOW5:
		log.Info("Starting NetFlow5 listener...")
		stateNFLegacy := &utils.StateNFLegacy{
			Format: ndmFlowDriver,
			Logger: logger,
		}
		flowState = stateNFLegacy
		go func() {
			err = stateNFLegacy.FlowRoutine(1, hostname, int(port), reusePort)
			if err != nil {
				log.Errorf("Error listener to netflow5: %s", err)
			}
		}()
	default:
		return Listener{}, fmt.Errorf("unknown flow type: %s", listenerConfig.FlowType)
	}
	listener := Listener{
		flowState: flowState,
		config:    listenerConfig,
	}
	return listener, nil
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
