// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package windowseventlogimpl provides the Windows Event Log check component
package windowseventlogimpl

import (
	"context"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/checks/windowseventlog"
	check "github.com/DataDog/datadog-agent/comp/checks/windowseventlog/windowseventlogimpl/check"
	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newComp),
	)
}

type dependencies struct {
	fx.In

	// Logs Agent component, used to send integration logs
	// It is optional because the Logs Agent can be disabled
	LogsComponent optional.Option[logsAgent.Component]
	Config        configComponent.Component

	Lifecycle fx.Lifecycle
}

func newComp(deps dependencies) windowseventlog.Component {
	deps.Lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			core.RegisterCheck(check.CheckName, check.Factory(deps.LogsComponent, deps.Config))
			return nil
		},
	})
	return struct{}{}
}
