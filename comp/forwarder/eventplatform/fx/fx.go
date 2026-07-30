// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx provides the fx module for the event platform forwarder component.
package fx

import (
	"go.uber.org/fx"

	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	eventplatformimpl "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-log-pipelines

// Module defines the fx options for this component.
func Module(params eventplatform.Params) fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			eventplatformimpl.NewComponent,
		),
		fx.Supply(params),
	)
}
