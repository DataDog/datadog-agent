// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package logsagentpipeline contains the logs agent pipeline component.
// See def/component.go for the component interface.
// See impl/agent.go for the implementation.
// See fx/fx.go for the fx module.
package logsagentpipeline

import defpkg "github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline/def"

// Component is the component type.
// Deprecated: Use github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline/def.Component instead.
type Component = defpkg.Component

// LogsAgent is a compat version of component for non fx usage.
// Deprecated: Use github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline/def.LogsAgent instead.
type LogsAgent = defpkg.LogsAgent
