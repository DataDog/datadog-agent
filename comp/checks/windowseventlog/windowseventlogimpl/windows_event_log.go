// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package windowseventlogimpl

import (
	"context"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/checks/windows_event_log"
	check "github.com/DataDog/datadog-agent/comp/checks/windows_event_log/windows_event_logimpl/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newComp),
	)
}

type dependencies struct {
	fx.In

	Lifecycle fx.Lifecycle
}

func newComp(deps dependencies) windowseventlog.Component {
	deps.Lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			core.RegisterCheck(check.CheckName, check.Factory())
			return nil
		},
	})
	return struct{}{}
}
