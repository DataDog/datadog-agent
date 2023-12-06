// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package impl

import (
	"context"
	"errors"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/ndmtmp/forwarder"
	"github.com/DataDog/datadog-agent/comp/snmpwalk/common"
	nfconfig "github.com/DataDog/datadog-agent/comp/snmpwalk/config"
	"github.com/DataDog/datadog-agent/comp/snmpwalk/fetch"
	"github.com/DataDog/datadog-agent/comp/snmpwalk/server"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"go.uber.org/fx"
	"sync"
	"time"
)

type dependencies struct {
	fx.In
	Config        nfconfig.Component
	Logger        log.Component
	Demultiplexer demultiplexer.Component
	Forwarder     forwarder.Component
	Hostname      hostname.Component
}

// TODO: (components)
// The Status command is not yet a component.
// Therefore, the globalServer variable below is used as a temporary workaround.
// globalServer is only used on getting the status of the server.
var (
	globalServer   = &Server{}
	globalServerMu sync.Mutex
)

// newServer configures a snmpwalk server.
func newServer(lc fx.Lifecycle, deps dependencies) (server.Component, error) {
	deps.Logger.Infof("[SNMPWALK] newServer")
	conf := deps.Config.Get()
	sender, err := deps.Demultiplexer.GetDefaultSender()
	if err != nil {
		return nil, err
	}

	runner := fetch.NewSnmpwalkRunner(sender)

	server := &Server{
		config: conf,
		logger: deps.Logger,
		runner: runner,
	}

	// TODO: USE SENDER
	_ = sender
	server.logger.Infof("[SNMPWALK] Starting Snmpwalk Server")

	ticker := time.NewTicker(10 * time.Second)
	quit := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				server.logger.Debugf("[SNMPWALK] Ticker")
				runner.Callback()
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()

	globalServerMu.Lock()
	globalServer = server
	globalServerMu.Unlock()

	if conf.Enabled {
		// snmpwalk is enabled, so start the server
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
	return server, nil
}

// Server manages snmpwalk.
type Server struct {
	Addr    string
	config  *common.SnmpwalkConfig
	logger  log.Component
	running bool
	runner  *fetch.SnmpwalkRunner
}

// Start starts the server running
func (s *Server) Start() error {
	if s.running {
		return errors.New("server already started")
	}
	s.running = true

	return nil
}

// Stop stops the Server.
func (s *Server) Stop() {
	s.logger.Infof("Stop Snmpwalk Server")
	if !s.running {
		return
	}

	s.runner.Stop()

	s.running = false
}

// IsEnabled checks if the snmpwalk functionality is enabled in the configuration.
func IsEnabled() bool {
	return pkgconfig.Datadog.GetBool("network_devices.snmpwalk.enabled")
}
