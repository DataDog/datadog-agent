// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"github.com/DataDog/datadog-agent/pkg/mcp/types"
)

// Re-export types for backwards compatibility
type (
	ToolRequest  = types.ToolRequest
	ToolResponse = types.ToolResponse
	Handler      = types.Handler
)
