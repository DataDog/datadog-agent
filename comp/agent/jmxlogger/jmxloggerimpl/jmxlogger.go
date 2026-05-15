// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package jmxloggerimpl implements the logger for JMX.
// Deprecated: use comp/agent/jmxlogger/impl instead.
package jmxloggerimpl

import (
	"go.uber.org/fx"

	jmxloggerimplnew "github.com/DataDog/datadog-agent/comp/agent/jmxlogger/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Params defines the parameters for the JMX logger.
// Deprecated: use comp/agent/jmxlogger/impl.Params instead.
type Params = jmxloggerimplnew.Params

// NewCliParams creates a new Params for CLI usage.
// Deprecated: use comp/agent/jmxlogger/impl.NewCliParams instead.
var NewCliParams = jmxloggerimplnew.NewCliParams

// NewDefaultParams creates a new Params with default values.
// Deprecated: use comp/agent/jmxlogger/impl.NewDefaultParams instead.
var NewDefaultParams = jmxloggerimplnew.NewDefaultParams

// Module defines the fx options for this component.
// Deprecated: use comp/agent/jmxlogger/fx instead.
func Module(params Params) fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			jmxloggerimplnew.NewComponent,
		),
		fx.Supply(params),
	)
}
