// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"go.uber.org/fx"
)

type Component interface {
	run() error
	stop() error
}

var Module = fxutil.Component(
	fx.Provide(
		fx.Annotate(
			newRunner,
			fx.OnStart(func(c Component) error { return c.run() }),
			fx.OnStop(func(c Component) error { return c.stop() }),
		),
	),
)
