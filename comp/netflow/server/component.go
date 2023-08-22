// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package server implements a component that runs the netflow observation server.
package server

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"go.uber.org/fx"

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

// Mock implements mock-specific methods.
type Mock interface {
	Component
}

type dependencies struct {
	fx.In
	Config config.Component
	Logger log.Component
	Demux  aggregator.DemultiplexerWithAggregator
}

func newServer(dep dependencies) (Component, error) {
	epForwarder, err := dep.Demux.GetEventPlatformForwarder()
	if err != nil {
		return nil, err
	}

	sender, err := dep.Demux.GetDefaultSender()
	if err != nil {
		return nil, err
	}
	return netflow.NewNetflowServer(sender, epForwarder, dep.Config, dep.Logger)
}
