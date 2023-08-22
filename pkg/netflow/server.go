// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

// Package netflow defines listeners that parse metrics and events from netflow traffic
package netflow

import (
	"context"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"

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
	logger    log.Component
}

// NewNetflowServer configures and returns a running SNMP traps server.
func NewNetflowServer(sender sender.Sender, epForwarder epforwarder.EventPlatformForwarder, ddconf ddconfig.ConfigReader, logger log.Component) (*Server, error) {
	var listeners []*netflowListener

	mainConfig, err := config.ReadConfig(ddconf)
	if err != nil {
		return nil, err
	}

	hostnameDetected, err := hostname.Get(context.TODO())
	if err != nil {
		logger.Warnf("Error getting the hostname: %v", err)
		hostnameDetected = ""
	}

	flowAgg := flowaggregator.NewFlowAggregator(sender, epForwarder, mainConfig, hostnameDetected, logger)
	go flowAgg.Start()

	if mainConfig.PrometheusListenerEnabled {
		go func() {
			serverMux := http.NewServeMux()
			serverMux.Handle("/metrics", promhttp.Handler())
			err := http.ListenAndServe(mainConfig.PrometheusListenerAddress, serverMux)
			if err != nil {
				logger.Errorf("error starting prometheus server `%s`", mainConfig.PrometheusListenerAddress)
			}
		}()
	}

	logger.Debugf("NetFlow Server configs (aggregator_buffer_size=%d, aggregator_flush_interval=%d, aggregator_flow_context_ttl=%d)", mainConfig.AggregatorBufferSize, mainConfig.AggregatorFlushInterval, mainConfig.AggregatorFlowContextTTL)
	for _, listenerConfig := range mainConfig.Listeners {
		logger.Infof("Starting Netflow listener for flow type %s on %s", listenerConfig.FlowType, listenerConfig.Addr())
		listener, err := startFlowListener(listenerConfig, flowAgg, logger)
		if err != nil {
			logger.Warnf("Error starting listener for config (flow_type:%s, bind_Host:%s, port:%d): %s", listenerConfig.FlowType, listenerConfig.BindHost, listenerConfig.Port, err)
			continue
		}
		listeners = append(listeners, listener)
	}

	return &Server{
		listeners: listeners,
		config:    mainConfig,
		flowAgg:   flowAgg,
		logger:    logger,
	}, nil
}

// Stop stops the Server.
func (s *Server) stop() {
	s.logger.Infof("Stop NetFlow Server")

	s.flowAgg.Stop()

	for _, listener := range s.listeners {
		stopped := make(chan interface{})

		go func() {
			s.logger.Infof("Listener `%s` shutting down", listener.config.Addr())
			listener.shutdown()
			close(stopped)
		}()

		select {
		case <-stopped:
			s.logger.Infof("Listener `%s` stopped", listener.config.Addr())
		case <-time.After(time.Duration(s.config.StopTimeout) * time.Second):
			s.logger.Errorf("Stopping listener `%s`. Timeout after %d seconds", listener.config.Addr(), s.config.StopTimeout)
		}
	}
}

// StartServer starts the global NetFlow collector.
func StartServer(demux aggregator.DemultiplexerWithAggregator, ddconf ddconfig.ConfigReader, logger log.Component) error {
	epForwarder, err := demux.GetEventPlatformForwarder()
	if err != nil {
		return err
	}

	sender, err := demux.GetDefaultSender()
	if err != nil {
		return err
	}
	server, err := NewNetflowServer(sender, epForwarder, ddconf, logger)
	if err != nil {
		return err
	}
	serverInstance = server
	return nil
}

// StopServer stops the netflow server, if it is running.
func StopServer() {
	if serverInstance != nil {
		serverInstance.stop()
		serverInstance = nil
	}
}

// IsEnabled returns whether NetFlow collection is enabled in the Agent configuration.
func IsEnabled(ddconf ddconfig.ConfigReader) bool {
	return ddconf.GetBool("network_devices.netflow.enabled")
}
