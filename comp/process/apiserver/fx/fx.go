// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx provides the fx module for the process apiserver component
package fx

import (
	apiserverimpl "github.com/DataDog/datadog-agent/comp/process/apiserver/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(fxutil.ProvideComponentConstructor(apiserverimpl.NewAPIServer)))
}
