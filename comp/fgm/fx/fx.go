// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx provides the fx module for the FGM component
package fx

import (
	"go.uber.org/fx"

	fgmdef "github.com/DataDog/datadog-agent/comp/fgm/def"
	fgmimpl "github.com/DataDog/datadog-agent/comp/fgm/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-metrics-logs

// Module defines the fx options for the fgm component
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(fgmimpl.NewComponent),
		// Invoke to ensure the component is instantiated even though nothing depends on it
		fx.Invoke(func(_ fgmdef.Component) {}),
	)
}
