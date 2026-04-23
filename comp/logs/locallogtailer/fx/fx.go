// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx provides the fx module for the localreader component.
package fx

import (
	locallogtailerdef "github.com/DataDog/datadog-agent/comp/logs/locallogtailer/def"
	locallogtailerimpl "github.com/DataDog/datadog-agent/comp/logs/locallogtailer/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			locallogtailerimpl.NewComponent,
		),
		// Force eager instantiation: locallogtailerdef.Component is never
		// required by any other component in the graph, so without this
		// fx.Invoke Fx would never call NewComponent and the tailer would
		// silently never start.
		fx.Invoke(func(_ locallogtailerdef.Component) {}),
	)
}
