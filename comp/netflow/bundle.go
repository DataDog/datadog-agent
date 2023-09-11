// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package netflow implements the "netflow" bundle, which listens for netflow
// packets, processes them, and forwards relevant data to the backend.
package netflow

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/netflow/config"
	"github.com/DataDog/datadog-agent/comp/netflow/forwarder"
	"github.com/DataDog/datadog-agent/comp/netflow/hostname"
	"github.com/DataDog/datadog-agent/comp/netflow/sender"
	"github.com/DataDog/datadog-agent/comp/netflow/server"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// team: network-device-monitoring

// Bundle defines the fx options for this bundle.
var Bundle = fxutil.Bundle(
	server.Module,
	config.Module,
	sender.Module,
	forwarder.Module,
	hostname.Module,
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
