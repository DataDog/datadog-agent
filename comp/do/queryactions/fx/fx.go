// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fx provides the fx module for the DO query actions component
package fx

import (
	doqueryactions "github.com/DataDog/datadog-agent/comp/do/queryactions/def"
	doqueryactionsimpl "github.com/DataDog/datadog-agent/comp/do/queryactions/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			doqueryactionsimpl.NewComponent,
		),
		// Force instantiation since nothing depends on this component
		fx.Invoke(func(_ doqueryactions.Component) {}),
	)
}
