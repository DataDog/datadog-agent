// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package netflow

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/hostname"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/netflow/config"
	"github.com/DataDog/datadog-agent/pkg/netflow/flowaggregator"
)

var serverInstance *Server

// Server manages netflow listeners.
type Server struct {
	Addr      string
	config    *config.NetflowConfig
	listeners []*netflowListener
	flowAgg   *flowaggregator.FlowAggregator
}

// NewNetflowServer configures and returns a running SNMP traps server.
func NewNetflowServer(sender aggregator.Sender) (*Server, error) {
	var listeners []*netflowListener

	mainConfig, err := config.ReadConfig()
	if err != nil {
		return nil, err
	}

	hostnameDetected, err := hostname.Get(context.TODO())
	if err != nil {
		log.Warnf("Error getting the hostname: %v", err)
		hostnameDetected = ""
	}

	flowAgg := flowaggregator.NewFlowAggregator(sender, mainConfig, hostnameDetected)
	go flowAgg.Start()

	for _, listenerConfig := range mainConfig.Listeners {
		log.Infof("Starting Netflow listener for flow type %s on %s", listenerConfig.FlowType, listenerConfig.Addr())
		listener, err := startFlowListener(listenerConfig, flowAgg)
		if err != nil {
			log.Warnf("Error starting listener for config (flow_type:%s, bind_Host:%s, port:%d): %s", listenerConfig.FlowType, listenerConfig.BindHost, listenerConfig.Port, err)
			continue
		}
		listeners = append(listeners, listener)
	}

	return &Server{
		listeners: listeners,
		config:    mainConfig,
		flowAgg:   flowAgg,
	}, nil
}

// Stop stops the Server.
func (s *Server) stop() {
	log.Infof("Stop NetFlow Server")

	s.flowAgg.Stop()

	for _, listener := range s.listeners {
		stopped := make(chan interface{})

		go func() {
			log.Infof("Listener `%s` shutting down", listener.config.Addr())
			listener.shutdown()
			close(stopped)
		}()

		select {
		case <-stopped:
			log.Infof("Listener `%s` stopped", listener.config.Addr())
		case <-time.After(time.Duration(s.config.StopTimeout) * time.Second):
			log.Errorf("Stopping listener `%s`. Timeout after %d seconds", listener.config.Addr(), s.config.StopTimeout)
		}
	}
}

// StartServer starts the global NetFlow collector.
func StartServer(sender aggregator.Sender) error {
	server, err := NewNetflowServer(sender)
	serverInstance = server
	return err
}

// StopServer stops the netflow server, if it is running.
func StopServer() {
	if serverInstance != nil {
		serverInstance.stop()
		serverInstance = nil
	}
}

// IsEnabled returns whether NetFlow collection is enabled in the Agent configuration.
func IsEnabled() bool {
	return coreconfig.Datadog.GetBool("network_devices.netflow.enabled")
}
