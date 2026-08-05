// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package logsagentpipelineimpl contains the implementation for the logs agent pipeline component
package logsagentpipelineimpl

import (
	logsagentpipelinedef "github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline/def"
	logsagentpipelinefx "github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline/fx"
	implpkg "github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
// Deprecated: use github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline/fx.Module instead.
func Module() fxutil.Module {
	return logsagentpipelinefx.Module()
}

// Dependencies specifies the list of dependencies needed to initialize the logs agent.
// Deprecated: use github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline/impl.Dependencies instead.
type Dependencies = implpkg.Dependencies

// NewLogsAgent returns a new instance of the logs agent with the given dependencies.
// Deprecated: use github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline/impl.NewLogsAgent instead.
func NewLogsAgent(deps Dependencies) logsagentpipelinedef.LogsAgent {
	return implpkg.NewLogsAgent(deps)
}
