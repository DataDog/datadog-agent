// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package server

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/ndmtmp/forwarder"
	nfconfig "github.com/DataDog/datadog-agent/comp/netflow/config"
	"github.com/DataDog/datadog-agent/comp/netflow/flowaggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
)

type dependencies struct {
	fx.In
	Config        nfconfig.Component
	Logger        log.Component
	Demultiplexer demultiplexer.Component
	Forwarder     forwarder.Component
	Hostname      hostname.Component
}

type provides struct {
	fx.Out

	Comp           Component
	StatusProvider status.InformationProvider
}

// NetflowServerStatus represents the status of the server including details about
// listeners which are working and those which have closed.
type NetflowServerStatus struct {
	TotalListeners         int
	OpenListeners          int
	ClosedListeners        int
	WorkingListenerDetails []NetflowListenerStatus
	ClosedListenerDetails  []NetflowListenerStatus
}

// NetflowListenerStatus handles logic related to pulling config information and associating it to an error.
type NetflowListenerStatus struct {
	Config    nfconfig.ListenerConfig
	Error     string
	FlowCount int64
}

// TODO: (components)
// The Status command is not yet a component.
// Therefore, the globalServer variable below is used as a temporary workaround.
// globalServer is only used on getting the status of the server.
var (
	globalServer   = &Server{}
	globalServerMu sync.Mutex
)

// newServer configures a netflow server.
func newServer(lc fx.Lifecycle, deps dependencies) (provides, error) {
	conf := deps.Config.Get()
	sender, err := deps.Demultiplexer.GetDefaultSender()
	if err != nil {
		return provides{}, err
	}
	flowAgg := flowaggregator.NewFlowAggregator(sender, deps.Forwarder, conf, deps.Hostname.GetSafe(context.Background()), deps.Logger)

	server := &Server{
		config:  conf,
		FlowAgg: flowAgg,
		logger:  deps.Logger,
	}

	globalServerMu.Lock()
	globalServer = server
	globalServerMu.Unlock()

	var statusProvider status.Provider

	statusProvider = status.NoopProvider{}

	if conf.Enabled {
		statusProvider = Provider{}

		// netflow is enabled, so start the server
		lc.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {

				err := server.Start()
				return err
			},
			OnStop: func(context.Context) error {
				server.Stop()
				return nil
			},
		})
	}
	return provides{
		Comp:           server,
		StatusProvider: status.NewInformationProvider(statusProvider),
	}, nil
}

// Server manages netflow listeners.
type Server struct {
	Addr      string
	config    *nfconfig.NetflowConfig
	listeners []*netflowListener
	FlowAgg   *flowaggregator.FlowAggregator
	logger    log.Component
	running   bool
}

// Start starts the server running
func (s *Server) Start() error {
	if s.running {
		return errors.New("server already started")
	}
	s.running = true
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
	if !s.running {
		return
	}
	s.FlowAgg.Stop()

	for _, listener := range s.listeners {
		stopped := make(chan interface{})

		go func() {
			s.logger.Infof("Listener `%s` shutting down", listener.config.Addr())
			close(stopped)
		}()

		select {
		case <-stopped:
			s.logger.Infof("Listener `%s` stopped", listener.config.Addr())
		case <-time.After(time.Duration(s.config.StopTimeout) * time.Second):
			s.logger.Errorf("Stopping listener `%s`. Timeout after %d seconds", listener.config.Addr(), s.config.StopTimeout)
		}
	}
	s.running = false
}

// IsEnabled checks if the netflow functionality is enabled in the configuration.
func IsEnabled() bool {
	return config.Datadog.GetBool("network_devices.netflow.enabled")
}

// GetStatus retrieves the current status of the server with details about
// all listeners and categorizes them into working and closed.
func GetStatus() NetflowServerStatus {
	globalServerMu.Lock()
	defer globalServerMu.Unlock()

	if globalServer == nil {
		return NetflowServerStatus{}
	}

	workingListeners := []NetflowListenerStatus{}
	closedListenersList := []NetflowListenerStatus{}

	for _, listener := range globalServer.listeners {
		errorString := listener.error.Load()
		if errorString != "" {
			closedListenersList = append(closedListenersList, NetflowListenerStatus{
				Config: listener.config,
				Error:  errorString,
			})
		} else {
			workingListeners = append(workingListeners, NetflowListenerStatus{
				Config:    listener.config,
				FlowCount: listener.flowCount.Load(),
			})
		}
	}

	return NetflowServerStatus{
		TotalListeners:         int(len(globalServer.listeners)),
		OpenListeners:          int(len(workingListeners)),
		ClosedListeners:        int(len(closedListenersList)),
		WorkingListenerDetails: workingListeners,
		ClosedListenerDetails:  closedListenersList,
	}
}
