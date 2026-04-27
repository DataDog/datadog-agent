// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

// Package serverimpl implements the traps server.
package serverimpl

import (
	"context"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	coreStatus "github.com/DataDog/datadog-agent/comp/core/status"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	server "github.com/DataDog/datadog-agent/comp/snmptraps/server/def"

	trapsconfig "github.com/DataDog/datadog-agent/comp/snmptraps/config/def"
	configfx "github.com/DataDog/datadog-agent/comp/snmptraps/config/fx"
	formatter "github.com/DataDog/datadog-agent/comp/snmptraps/formatter/def"
	formatterimpl "github.com/DataDog/datadog-agent/comp/snmptraps/formatter/impl"
	forwarder "github.com/DataDog/datadog-agent/comp/snmptraps/forwarder/def"
	forwarderimpl "github.com/DataDog/datadog-agent/comp/snmptraps/forwarder/fx"
	listener "github.com/DataDog/datadog-agent/comp/snmptraps/listener/def"
	listenerfx "github.com/DataDog/datadog-agent/comp/snmptraps/listener/fx"
	oidresolverfx "github.com/DataDog/datadog-agent/comp/snmptraps/oidresolver/fx"
	"github.com/DataDog/datadog-agent/comp/snmptraps/status/def"
	statusimpl "github.com/DataDog/datadog-agent/comp/snmptraps/status/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil/logging"
)

// Requires defines the dependencies for the server component.
type Requires struct {
	compdef.In
	Lc        compdef.Lifecycle
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

// Provides defines the output of the server component.
type Provides struct {
	compdef.Out

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

// NewComponent creates a new traps server, registering it with the lifecycle
// system if traps are enabled.
func NewComponent(deps Requires) Provides {
	if !trapsconfig.IsEnabled(deps.Conf) {
		return Provides{
			Comp: &TrapsServer{running: false},
		}
	}
	stat := statusimpl.New()
	// TODO: (components) Having apps within apps is not ideal - you have to be
	// careful never to double-instantiate anything. Do not use this solution
	// elsewhere if possible.
	app := fx.New(
		logging.DefaultFxLoggingOption(),
		fxutil.FxLifecycleAdapter(),
		fx.Supply(injections{
			Conf:      deps.Conf,
			HNService: deps.HNService,
			Demux:     deps.Demux,
			Logger:    deps.Logger,
			Status:    stat,
		}),
		configfx.Module(),
		fxutil.ProvideComponentConstructor(formatterimpl.NewComponent),
		fxutil.ProvideOptional[formatter.Component](),
		forwarderimpl.Module(),
		listenerfx.Module(),
		oidresolverfx.Module(),
		fx.Invoke(func(_ forwarder.Component, _ listener.Component) {}),
	)
	srv := &TrapsServer{app: app, stat: stat}

	if err := app.Err(); err != nil {
		deps.Logger.Errorf("Failed to initialize snmp-traps server: %s", err)
		srv.stat.SetStartError(err)
		return Provides{
			Comp: srv,
		}
	}

	deps.Lc.Append(compdef.Hook{
		OnStart: func(ctx context.Context) error {
			err := app.Start(ctx)
			if err != nil {
				deps.Logger.Errorf("Failed to start snmp-traps server: %s", err)
				srv.stat.SetStartError(err)
			} else {
				srv.running = true
			}
			return nil
		},
		OnStop: func(ctx context.Context) error {
			srv.running = false
			return app.Stop(ctx)
		},
	})

	return Provides{
		Comp:           srv,
		StatusProvider: coreStatus.NewInformationProvider(statusimpl.Provider{}),
	}
}
