// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package server implements a component that runs the netflow server.
// When running, it listens for network traffic according to configured
// listeners and aggregates traffic data to send to the backend.
// It does not expose any public methods.
package server

import (
	"context"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/netflow"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
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

type dependencies struct {
	fx.In
	Demux  *aggregator.AgentDemultiplexer
	Conf   config.Component
	Logger log.Component
}

func newServer(lc fx.Lifecycle, dep dependencies) (Component, error) {
	epForwarder, err := dep.Demux.GetEventPlatformForwarder()
	if err != nil {
		return nil, err
	}

	sender, err := dep.Demux.GetDefaultSender()
	if err != nil {
		return nil, err
	}
	server, err := netflow.NewNetflowServer(sender, epForwarder, dep.Conf, dep.Logger)
	if err != nil {
		return nil, err
	}
	if netflow.IsEnabled(dep.Conf) {
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
	return server, nil
}
