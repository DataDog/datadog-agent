// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package payloadmodifier defines the trace payload modifier component interface
package payloadmodifier

import (
	pkgagent "github.com/DataDog/datadog-agent/pkg/trace/agent"
)

// team: agent-apm

// Component provides trace payload modification functionality
type Component interface {
	// GetModifier returns the TracerPayloadModifier instance
	// Returns nil if payload modification is not enabled
	GetModifier() pkgagent.TracerPayloadModifier
}
