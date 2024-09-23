// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fx provides the fx module for the datadogclient component
package fx

import (
	datadogclient "github.com/DataDog/datadog-agent/comp/autoscaling/datadogclient/def"
	datadogclientimpl "github.com/DataDog/datadog-agent/comp/autoscaling/datadogclient/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"go.uber.org/fx"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			datadogclientimpl.NewComponent,
		),
		fx.Provide(func(c datadogclient.Component) optional.Option[datadogclient.Component] {
			if _, ok := c.(*datadogclientimpl.ImplNone); ok {
				return optional.NewNoneOption[datadogclient.Component]()
			}
			return optional.NewOption[datadogclient.Component](c)
		}),
	)
}
