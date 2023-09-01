// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/netflow/config"
	"github.com/DataDog/datadog-agent/pkg/netflow/flowaggregator"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/fx"
)

// We expose these types internally in order to inject them; if they are ever componentized separately we will use the true componentized versions.
type netflowHostname string
type netflowEPForwarder epforwarder.EventPlatformForwarder
type netflowSender sender.Sender

func getHostname(logger log.Component) netflowHostname {
	host, err := hostname.Get(context.Background())
	if err != nil {
		logger.Warnf("Error getting the hostname: %v", err)
		return ""
	}
	return netflowHostname(host)
}

func getNetflowSender(agg aggregator.DemultiplexerWithAggregator) (netflowSender, error) {
	return agg.GetDefaultSender()
}

func getNetflowForwarder(agg aggregator.DemultiplexerWithAggregator) (netflowEPForwarder, error) {
	return agg.GetEventPlatformForwarder()
}

type dependencies struct {
	fx.In
	Config      *config.NetflowConfig
	Logger      log.Component
	EPForwarder netflowEPForwarder
	Sender      netflowSender
	Hostname    netflowHostname
}

// newServer configures a netflow server.
func newServer(lc fx.Lifecycle, dep dependencies) (Component, error) {
	if !dep.Config.Enabled {
		// no-op
		return nil, nil
	}
	flowAgg := flowaggregator.NewFlowAggregator(dep.Sender, dep.EPForwarder, dep.Config, string(dep.Hostname), dep.Logger)

	server := &Server{
		config:  dep.Config,
		FlowAgg: flowAgg,
		logger:  dep.Logger,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return server.Start()
		},
		OnStop: func(context.Context) error {
			server.Stop()
			return nil
		},
	})
	return server, nil
}

// Server manages netflow listeners.
type Server struct {
	Addr      string
	config    *config.NetflowConfig
	listeners []*netflowListener
	FlowAgg   *flowaggregator.FlowAggregator
	logger    log.Component
	started   bool
}

// Start starts the server running
func (s *Server) Start() error {
	if s.started {
		return errors.New("server already started")
	}
	s.started = true
	go s.FlowAgg.Start()

	if s.config.PrometheusListenerEnabled {
		go func() {
			serverMux := http.NewServeMux()
			serverMux.Handle("/metrics", promhttp.Handler())
			err := http.ListenAndServe(s.config.PrometheusListenerAddress, serverMux)
			if err != nil {
				s.logger.Errorf("error starting prometheus server `%s`", s.config.PrometheusListenerAddress)
			}
		}()
	}
	s.logger.Debugf("NetFlow Server configs (aggregator_buffer_size=%d, aggregator_flush_interval=%d, aggregator_flow_context_ttl=%d)", s.config.AggregatorBufferSize, s.config.AggregatorFlushInterval, s.config.AggregatorFlowContextTTL)
	for _, listenerConfig := range s.config.Listeners {
		s.logger.Infof("Starting Netflow listener for flow type %s on %s", listenerConfig.FlowType, listenerConfig.Addr())
		listener, err := startFlowListener(listenerConfig, s.FlowAgg, s.logger)
		if err != nil {
			s.logger.Warnf("Error starting listener for config (flow_type:%s, bind_Host:%s, port:%d): %s", listenerConfig.FlowType, listenerConfig.BindHost, listenerConfig.Port, err)
			continue
		}
		s.listeners = append(s.listeners, listener)
	}
	return nil
}

// Stop stops the Server.
func (s *Server) Stop() {
	s.logger.Infof("Stop NetFlow Server")

	s.FlowAgg.Stop()

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
