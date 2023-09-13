// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package snmptraps implements the a server that listens for SNMP trap data
// and sends it to the backend.
package snmptraps

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/snmptraps/config"
	"github.com/DataDog/datadog-agent/comp/snmptraps/formatter"
	"github.com/DataDog/datadog-agent/comp/snmptraps/forwarder"
	"github.com/DataDog/datadog-agent/comp/snmptraps/listener"
	"github.com/DataDog/datadog-agent/comp/snmptraps/oid_resolver"
	"github.com/DataDog/datadog-agent/comp/snmptraps/server"
	"github.com/DataDog/datadog-agent/comp/snmptraps/status"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// team: network-device-monitoring

// Bundle defines the fx options for this bundle.
var Bundle = fxutil.Bundle(
	config.Module,
	formatter.Module,
	forwarder.Module,
	listener.Module,
	oid_resolver.Module,
	status.Module,
	server.Module,
	// Run the server
	fx.Invoke(func(lc fx.Lifecycle, server server.Component, conf config.Component) {
		if !conf.Get().Enabled {
			// netflow is disabled - don't do anything.
			return
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

	}),
)
