// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package logsagentpipelineimpl contains the implementation for the logs agent pipeline component
package logsagentpipelineimpl

import (
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"go.uber.org/fx"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(NewLogsAgentComponent))
}
