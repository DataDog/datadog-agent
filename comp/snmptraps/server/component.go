// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package server implements a component that runs the traps server.
// It listens for SNMP trap messages on a configured port, parses and
// reformats them, and sends the resulting data to the backend.
package server

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: network-device-monitoring

// Component is the component type.
type Component interface{}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newServer),
)
