// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

// Package serverimpl implements the traps server.
package serverimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	"github.com/DataDog/datadog-agent/comp/core/log"
	coreStatus "github.com/DataDog/datadog-agent/comp/core/status"

	"go.uber.org/fx"

	trapsconfig "github.com/DataDog/datadog-agent/comp/snmptraps/config"
	"github.com/DataDog/datadog-agent/comp/snmptraps/config/configimpl"
	"github.com/DataDog/datadog-agent/comp/snmptraps/formatter/formatterimpl"
	"github.com/DataDog/datadog-agent/comp/snmptraps/forwarder"
	"github.com/DataDog/datadog-agent/comp/snmptraps/forwarder/forwarderimpl"
	"github.com/DataDog/datadog-agent/comp/snmptraps/listener"
	"github.com/DataDog/datadog-agent/comp/snmptraps/listener/listenerimpl"
	"github.com/DataDog/datadog-agent/comp/snmptraps/oidresolver/oidresolverimpl"
	"github.com/DataDog/datadog-agent/comp/snmptraps/server"
	"github.com/DataDog/datadog-agent/comp/snmptraps/status"
	"github.com/DataDog/datadog-agent/comp/snmptraps/status/statusimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newServer),
)

type dependencies struct {
	fx.In
	Conf      config.Component
	HNService hostname.Component
	Demux     demultiplexer.Component
	Logger    log.Component
}

// injections bundles the injectables passed from the main app to the subapp.
type injections struct {
	fx.Out
	Conf      config.Component
	HNService hostname.Component
	Demux     demultiplexer.Component
	Logger    log.Component
	Status    status.Component
}

type provides struct {
	fx.Out

	Comp           server.Component
	StatusProvider coreStatus.InformationProvider
}

// TrapsServer implements the SNMP traps service.
type TrapsServer struct {
	app     *fx.App
	running bool
	stat    status.Component
}

// Running indicates whether the traps server is currently running.
func (w *TrapsServer) Running() bool {
	return w.running
}

// Error reports any error from server initialization/startup.
func (w *TrapsServer) Error() error {
	if w.stat == nil {
		return nil
	}
	return w.stat.GetStartError()
}

// newServer creates a new traps server, registering it with the fx lifecycle
// system if traps are enabled.
func newServer(lc fx.Lifecycle, deps dependencies) provides {
	if !trapsconfig.IsEnabled(deps.Conf) {
		return provides{
			Comp:           &TrapsServer{running: false},
			StatusProvider: coreStatus.NoopInformationProvider(),
		}
	}
	stat := statusimpl.New()
	// TODO: (components) Having apps within apps is not ideal - you have to be
	// careful never to double-instantiate anything. Do not use this solution
	// elsewhere if possible.
	app := fx.New(
		fx.Supply(injections{
			Conf:      deps.Conf,
			HNService: deps.HNService,
			Demux:     deps.Demux,
			Logger:    deps.Logger,
			Status:    stat,
		}),
		configimpl.Module,
		formatterimpl.Module,
		forwarderimpl.Module,
		listenerimpl.Module,
		oidresolverimpl.Module,
		fx.Invoke(func(_ forwarder.Component, _ listener.Component) {}),
	)
	server := &TrapsServer{app: app, stat: stat}

	if err := app.Err(); err != nil {
		deps.Logger.Errorf("Failed to initialize snmp-traps server: %s", err)
		server.stat.SetStartError(err)
		return provides{
			Comp:           server,
			StatusProvider: coreStatus.NoopInformationProvider(),
		}
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			err := app.Start(ctx)
			if err != nil {
				deps.Logger.Errorf("Failed to start snmp-traps server: %s", err)
				server.stat.SetStartError(err)
			} else {
				server.running = true
			}
			return nil
		},
		OnStop: func(ctx context.Context) error {
			server.running = false
			return app.Stop(ctx)
		},
	})

	return provides{
		Comp:           server,
		StatusProvider: coreStatus.NewInformationProvider(statusimpl.Provider{}),
	}
}
