// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package logsagentpipeline

import (
	def "github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline/def"
)

// Component is the component type.
// Deprecated: use github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline/def.Component instead.
type Component = def.Component

// LogsAgent is a compat version of component for non fx usage.
// Deprecated: use github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline/def.LogsAgent instead.
type LogsAgent = def.LogsAgent
