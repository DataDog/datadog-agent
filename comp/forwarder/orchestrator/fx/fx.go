// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package fx provides the fx module for the orchestrator forwarder component.
package fx

import (
	"go.uber.org/fx"

	orchestrator "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/def"
	orchestratorimpl "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module(params orchestrator.Params) fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(orchestratorimpl.NewComponent),
		fx.Supply(params),
	)
}
