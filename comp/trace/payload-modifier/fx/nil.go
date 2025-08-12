// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx provides fx options for the payload-modifier component.
package fx

import (
	"go.uber.org/fx"

	pkgagent "github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// NilModule provides a nil TracerPayloadModifier for contexts that need to satisfy
// the dependency but don't require payload modification functionality.
func NilModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(func() pkgagent.TracerPayloadModifier { return nil }))
}

