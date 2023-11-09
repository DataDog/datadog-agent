// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

// Package serverimpl implements the traps server.
package serverimpl

import (
	"github.com/DataDog/datadog-agent/comp/snmptraps/forwarder"
	"github.com/DataDog/datadog-agent/comp/snmptraps/listener"
	"github.com/DataDog/datadog-agent/comp/snmptraps/server"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newServer),
)

// Server manages an SNMP trap listener.
type Server struct {
	listener  listener.Component
	forwarder forwarder.Component
}

// newServer configures a netflow server.
// Since the listener and forwarder are already registered with the lifecycle
// tracker, this component really just exists so that there's one component to
// register instead of requiring users to remember to depend on both the
// listener and the forwarder.
func newServer(l listener.Component, f forwarder.Component) server.Component {
	return &Server{
		listener:  l,
		forwarder: f,
	}
}
