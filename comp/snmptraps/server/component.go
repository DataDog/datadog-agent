// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package server implements a component that runs the traps server.
// It listens for SNMP trap messages on a configured port, parses and
// reformats them, and sends the resulting data to the backend.
package server

import (
	"context"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/snmp/traps"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
)

// team: network-device-monitoring

// Component is the component type.
type Component interface {
	Start() error
	Stop()
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newServer),
)

type server struct {
	hostname string
	demux    *aggregator.AgentDemultiplexer
	conf     config.Component
	logger   log.Component
}

func (s *server) Start() error {
	return traps.StartServer(s.hostname, s.demux, s.conf, s.logger)
}

func (s *server) Stop() {
	traps.StopServer()
}

type dependencies struct {
	fx.In
	Demux  *aggregator.AgentDemultiplexer
	Conf   config.Component
	Logger log.Component
}

func newServer(lc fx.Lifecycle, dep dependencies) (Component, error) {
	name, err := hostname.Get(context.Background())
	if err != nil {
		return nil, err
	}
	s := &server{
		hostname: name,
		demux:    dep.Demux,
		conf:     dep.Conf,
		logger:   dep.Logger,
	}
	if traps.IsEnabled(dep.Conf) {
		lc.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				return s.Start()
			},
			OnStop: func(ctx context.Context) error {
				s.Stop()
				return nil
			},
		})
	}
	return s, nil
}
