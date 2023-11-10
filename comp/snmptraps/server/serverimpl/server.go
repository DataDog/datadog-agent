// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

// Package serverimpl implements the traps server.
package serverimpl

import (
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	"github.com/DataDog/datadog-agent/comp/core/log"

	trapsconfig "github.com/DataDog/datadog-agent/comp/snmptraps/config"
	"github.com/DataDog/datadog-agent/comp/snmptraps/config/configimpl"
	"github.com/DataDog/datadog-agent/comp/snmptraps/formatter/formatterimpl"
	"github.com/DataDog/datadog-agent/comp/snmptraps/forwarder"
	"github.com/DataDog/datadog-agent/comp/snmptraps/forwarder/forwarderimpl"
	"github.com/DataDog/datadog-agent/comp/snmptraps/listener"
	"github.com/DataDog/datadog-agent/comp/snmptraps/listener/listenerimpl"
	"github.com/DataDog/datadog-agent/comp/snmptraps/oidresolver/oidresolverimpl"
	"github.com/DataDog/datadog-agent/comp/snmptraps/server"
	"github.com/DataDog/datadog-agent/comp/snmptraps/status/statusimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
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

type injections struct {
	fx.Out
	Conf      config.Component
	HNService hostname.Component
	Demux     demultiplexer.Component
	Logger    log.Component
}

func mapDeps(deps dependencies) injections {
	return injections{
		Conf:      deps.Conf,
		HNService: deps.HNService,
		Demux:     deps.Demux,
		Logger:    deps.Logger,
	}
}

// newServer configures a netflow server.
func newServer(lc fx.Lifecycle, deps dependencies) (server.Component, error) {
	if !trapsconfig.IsEnabled(deps.Conf) {
		return nil, nil
	}
	app := fx.New(
		fx.Supply(mapDeps(deps)),
		configimpl.Module,
		formatterimpl.Module,
		forwarderimpl.Module,
		listenerimpl.Module,
		oidresolverimpl.Module,
		statusimpl.Module,
		fx.Invoke(func(_ forwarder.Component, _ listener.Component) {}),
	)
	if err := app.Err(); err != nil {
		deps.Logger.Errorf("Failed to initialize snmp-traps server: %s", err)
		return nil, nil
	}
	lc.Append(fx.Hook{
		OnStart: app.Start,
		OnStop:  app.Stop,
	})
	return app, nil
}
