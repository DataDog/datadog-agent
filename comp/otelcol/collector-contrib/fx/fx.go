// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package collectorcontribFx provides fx access for the collectorcontrib component
package collectorcontribfx

import (
	collectorcontrib "github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/def"
	collectorcontribimpl "github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(func() collectorcontrib.Component {
			// TODO: (agent-shared-components) use fxutil.ProvideComponentConstruct once it is implemented
			// See the RFC "fx-decoupled components" for more details
			return collectorcontribimpl.NewComponent()
		}))
}
