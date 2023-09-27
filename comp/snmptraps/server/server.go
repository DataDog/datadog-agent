// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package server

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/hostname"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/ndmtmp/sender"
	trapsconf "github.com/DataDog/datadog-agent/comp/snmptraps/config"
	"github.com/DataDog/datadog-agent/comp/snmptraps/forwarder"
	"github.com/DataDog/datadog-agent/comp/snmptraps/listener"
	"go.uber.org/fx"
)

// Server manages an SNMP trap listener.
type Server struct {
	Addr      string
	config    *trapsconf.TrapsConfig
	listener  listener.Component
	forwarder forwarder.Component
	logger    log.Component
}

type dependencies struct {
	fx.In
	Config    trapsconf.Component
	Listener  listener.Component
	Logger    log.Component
	Sender    sender.Component
	Forwarder forwarder.Component
	Hostname  hostname.Component
}

// newServer configures a netflow server.
func newServer(lc fx.Lifecycle, deps dependencies) Component {
	conf := deps.Config.Get()

	server := &Server{
		config:    conf,
		listener:  deps.Listener,
		forwarder: deps.Forwarder,
		logger:    deps.Logger,
	}
	if conf.Enabled {
		lc.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				return server.Start()
			},
			OnStop: func(ctx context.Context) error {
				server.Stop()
				return nil
			},
		})
	}
	return server
}

// Start starts the server listening
func (s *Server) Start() error {
	s.forwarder.Start()
	if err := s.listener.Start(); err != nil {
		s.forwarder.Stop()
		return err
	}
	return nil
}

// Stop stops the TrapServer.
func (s *Server) Stop() {
	stopped := make(chan interface{})

	go func() {
		s.logger.Infof("Stop listening on %s", s.config.Addr())
		s.listener.Stop()
		s.forwarder.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(time.Duration(s.config.StopTimeout) * time.Second):
		s.logger.Errorf("Stopping server. Timeout after %d seconds", s.config.StopTimeout)
	}
}
